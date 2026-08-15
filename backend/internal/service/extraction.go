package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/Hundsamurai/hopecore/backend/internal/fetcher"
	"github.com/Hundsamurai/hopecore/backend/internal/llm"
	"github.com/Hundsamurai/hopecore/backend/internal/model"
	"github.com/Hundsamurai/hopecore/backend/internal/store"
)

// Ошибки, которые HTTP-слой превращает в понятные ответы.
var (
	// ErrProviderUnavailable — провайдер не выбран, не настроен или без адаптера.
	ErrProviderUnavailable = errors.New("провайдер языковой модели недоступен")
	// ErrModelNotAllowed — модель не входит в список разрешённых конфигурацией.
	// Без этой проверки клиент мог бы потратить квоту на произвольную модель.
	ErrModelNotAllowed = errors.New("модель не разрешена конфигурацией")
	// ErrExtractionFailed — страница не скачалась или ответ не привели к схеме.
	ErrExtractionFailed = errors.New("извлечь данные не удалось")
)

// maxAttempts — сколько раз спрашиваем модель. Вторая попытка нужна провайдерам
// без строгой схемы: они гарантируют валидный JSON, но не соответствие схеме.
const maxAttempts = 2

// PageFetcher скачивает страницу вакансии. Интерфейс нужен, чтобы тесты
// не выходили в сеть.
type PageFetcher interface {
	Fetch(ctx context.Context, rawURL string) (fetcher.Page, error)
}

// Extraction — результат извлечения для предпросмотра.
//
// Вакансия при этом не меняется: запись идёт обычным PATCH после того, как
// пользователь отметит, что применять.
type Extraction struct {
	RunID    uint
	Provider string
	Model    string

	SourceURL   string
	SourceChars int
	// Warnings — про страницу: мало текста, антибот, обрезка по лимиту.
	Warnings []string

	// Values и Filled — что модель нашла и что прошло проверку.
	Values llm.Values
	Filled map[string]bool
	// Notes — почему отдельные поля отброшены.
	Notes []llm.FieldNote

	InputTokens  int
	OutputTokens int
	CostEstimate *float64
	Attempts     int
	DurationMs   int
}

// ExtractionService запускает извлечение полей вакансии моделью.
type ExtractionService struct {
	db        *gorm.DB
	fetcher   PageFetcher
	providers map[string]llm.Provider
	cfg       llm.Config
	log       *slog.Logger
	now       func() time.Time
}

// NewExtractionService собирает сервис.
func NewExtractionService(
	db *gorm.DB,
	pageFetcher PageFetcher,
	providers map[string]llm.Provider,
	cfg llm.Config,
	log *slog.Logger,
) *ExtractionService {
	return &ExtractionService{
		db:        db,
		fetcher:   pageFetcher,
		providers: providers,
		cfg:       cfg,
		log:       log,
		now:       func() time.Time { return time.Now().UTC() },
	}
}

// Enabled сообщает, есть ли хотя бы один настроенный провайдер.
func (s *ExtractionService) Enabled() bool {
	return len(s.providers) > 0
}

// Extract скачивает страницу вакансии и просит модель извлечь поля.
//
// Запись в журнал создаётся на всех путях, включая неудачные: видно, что
// попытка была, сколько она стоила и почему не получилось.
func (s *ExtractionService) Extract(ctx context.Context, vacancyID uint, providerID, modelName string) (Extraction, error) {
	vacancy, err := store.GetVacancy(s.db, vacancyID)
	if err != nil {
		return Extraction{}, err
	}

	providerCfg, ok := s.cfg.Provider(providerID)
	if !ok {
		return Extraction{}, fmt.Errorf("%w: %s", ErrProviderUnavailable, providerID)
	}
	adapter, ok := s.providers[providerID]
	if !ok {
		return Extraction{}, fmt.Errorf("%w: %s", ErrProviderUnavailable, providerID)
	}
	if modelName == "" {
		modelName = providerCfg.DefaultModel()
	}
	if !providerCfg.HasModel(modelName) {
		return Extraction{}, fmt.Errorf("%w: %s", ErrModelNotAllowed, modelName)
	}

	started := s.now()
	run := model.LLMRun{
		Purpose:       model.PurposeExtractVacancy,
		VacancyID:     &vacancy.ID,
		Provider:      providerID,
		Model:         modelName,
		PromptVersion: llm.PromptVersion,
		SourceURL:     vacancy.URL,
	}

	page, err := s.fetcher.Fetch(ctx, vacancy.URL)
	if err != nil {
		// Токены не потрачены, но запись всё равно нужна: пользователь увидит,
		// что попытка была и почему она ничего не стоила.
		run.Status = fetchErrorStatus(err)
		run.Error = err.Error()
		run.DurationMs = s.elapsed(started)
		s.saveRun(&run)

		s.log.Warn("страница вакансии не скачалась",
			"vacancy_id", vacancy.ID, "url", vacancy.URL, "error", err)
		return Extraction{}, fmt.Errorf("%w: %s", ErrExtractionFailed, err.Error())
	}

	run.SourceChars = page.Chars

	result, response, attempts, err := s.ask(ctx, adapter, modelName, vacancy.URL, page.Text)

	run.Attempts = attempts
	if response.InputTokens > 0 || response.OutputTokens > 0 {
		run.InputTokens = &response.InputTokens
		run.OutputTokens = &response.OutputTokens
		run.CostEstimate = providerCfg.Price.Estimate(response.InputTokens, response.OutputTokens)
	}
	if len(response.JSON) > 0 {
		run.ResponseJSON = string(response.JSON)
	}
	run.DurationMs = s.elapsed(started)

	if err != nil {
		run.Status = askErrorStatus(err)
		run.Error = err.Error()
		s.saveRun(&run)

		s.log.Warn("модель не дала пригодного ответа",
			"vacancy_id", vacancy.ID, "provider", providerID, "model", modelName, "error", err)
		return Extraction{}, fmt.Errorf("%w: %s", ErrExtractionFailed, err.Error())
	}

	run.Status = model.RunStatusOK
	s.saveRun(&run)

	s.log.Info("данные вакансии извлечены",
		"vacancy_id", vacancy.ID,
		"provider", providerID,
		"model", modelName,
		"source_chars", page.Chars,
		"filled", len(result.Filled),
		"notes", len(result.Notes),
		"input_tokens", response.InputTokens,
		"output_tokens", response.OutputTokens,
	)

	return Extraction{
		RunID:        run.ID,
		Provider:     providerID,
		Model:        modelName,
		SourceURL:    vacancy.URL,
		SourceChars:  page.Chars,
		Warnings:     page.Warnings,
		Values:       result.Values,
		Filled:       result.Filled,
		Notes:        result.Notes,
		InputTokens:  response.InputTokens,
		OutputTokens: response.OutputTokens,
		CostEstimate: run.CostEstimate,
		Attempts:     attempts,
		DurationMs:   run.DurationMs,
	}, nil
}

// ask спрашивает модель и разбирает ответ, повторяя запрос, если ответ вообще
// не содержал JSON. Возвращает последний ответ провайдера, чтобы потраченные
// токены попали в журнал даже при неудаче.
func (s *ExtractionService) ask(
	ctx context.Context,
	adapter llm.Provider,
	modelName, sourceURL, pageText string,
) (llm.Result, llm.Response, int, error) {
	request := llm.Request{
		Model:  modelName,
		System: llm.SystemPrompt,
		User:   llm.BuildUserPrompt(sourceURL, pageText),
		Schema: llm.ExtractionSchema(),
	}

	var (
		lastResponse llm.Response
		lastErr      error
	)

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		response, err := adapter.Complete(ctx, request)
		lastResponse = response

		if err != nil {
			// Ошибка провайдера повторной попытки не заслуживает: квота
			// и недоступность сами не пройдут за одну секунду.
			return llm.Result{}, response, attempt, err
		}

		result, parseErr := llm.Parse(response.JSON)
		if parseErr == nil {
			return result, response, attempt, nil
		}
		lastErr = parseErr

		if !errors.Is(parseErr, llm.ErrNoJSON) || attempt == maxAttempts {
			break
		}

		// Просим переделать, объяснив, что было не так.
		request.User = llm.BuildUserPrompt(sourceURL, pageText) + "\n\n" + llm.RepairPrompt(parseErr.Error())
		s.log.Warn("ответ модели не разобран, повторяем запрос", "model", modelName, "error", parseErr)
	}

	return llm.Result{}, lastResponse, maxAttempts, lastErr
}

func (s *ExtractionService) saveRun(run *model.LLMRun) {
	if err := store.CreateLLMRun(s.db, run); err != nil {
		// Журнал не должен ломать основную операцию: пользователь получит
		// результат, а о потере записи узнаем из логов.
		s.log.Error("не удалось записать запуск в журнал", "error", err)
	}
}

func (s *ExtractionService) elapsed(started time.Time) int {
	return int(s.now().Sub(started).Milliseconds())
}

// fetchErrorStatus различает таймаут и прочие неудачи скачивания:
// в журнале это разные истории.
func fetchErrorStatus(err error) string {
	var fetchErr *fetcher.Error
	if errors.As(err, &fetchErr) && fetchErr.Kind == fetcher.KindTimeout {
		return model.RunStatusTimeout
	}
	return model.RunStatusFetchError
}

// askErrorStatus отличает «модель ответила мусором» от «провайдер не ответил».
func askErrorStatus(err error) string {
	switch {
	case errors.Is(err, llm.ErrNoJSON):
		return model.RunStatusInvalidJSON
	case errors.Is(err, context.DeadlineExceeded):
		return model.RunStatusTimeout
	default:
		return model.RunStatusProviderError
	}
}
