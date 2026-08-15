package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Hundsamurai/hopecore/backend/internal/llm"
	"github.com/Hundsamurai/hopecore/backend/internal/model"
	"github.com/Hundsamurai/hopecore/backend/internal/store"
)

// providerResponse — провайдер в том виде, в каком его видит клиент.
// Ключа здесь нет и быть не может: наружу уходят только идентификатор,
// человеческое название и список разрешённых моделей.
type providerResponse struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Models       []string `json:"models"`
	DefaultModel string   `json:"default_model"`
	// PriceKnown говорит интерфейсу, показывать ли оценку стоимости
	// или ограничиться токенами.
	PriceKnown bool `json:"price_known"`
}

type providersResponse struct {
	Items []providerResponse `json:"items"`
}

// handleListProviders отдаёт провайдеров, которыми можно пользоваться.
//
// Пустой список — не ошибка: значит ключей нет, и приложение работает
// как на Этапе 1. Интерфейс по этому признаку выключает кнопку заполнения.
func (s *Server) handleListProviders(w http.ResponseWriter, _ *http.Request) {
	available := s.llm.Available()

	items := make([]providerResponse, 0, len(available))
	for _, provider := range available {
		items = append(items, newProviderResponse(provider))
	}

	writeJSON(w, http.StatusOK, providersResponse{Items: items})
}

func newProviderResponse(provider llm.ProviderConfig) providerResponse {
	return providerResponse{
		ID:           provider.ID,
		Label:        provider.Label,
		Models:       provider.Models,
		DefaultModel: provider.DefaultModel(),
		PriceKnown:   provider.Price.Known(),
	}
}

// llmRunResponse — запуск в списке. Сырого ответа модели здесь нет:
// он бывает большим и нужен только в карточке одного запуска.
type llmRunResponse struct {
	ID        uint      `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Purpose   string    `json:"purpose"`

	VacancyID *uint `json:"vacancy_id"`
	// VacancyTitle — подпись для экрана журнала. Пустая, если вакансию удалили:
	// запись о трате переживает карточку.
	VacancyTitle string `json:"vacancy_title"`

	Provider      string `json:"provider"`
	Model         string `json:"model"`
	PromptVersion string `json:"prompt_version"`

	SourceURL   string `json:"source_url"`
	SourceChars int    `json:"source_chars"`

	Status       string   `json:"status"`
	InputTokens  *int     `json:"input_tokens"`
	OutputTokens *int     `json:"output_tokens"`
	CostEstimate *float64 `json:"cost_estimate"`
	Attempts     int      `json:"attempts"`
	DurationMs   int      `json:"duration_ms"`
	Error        string   `json:"error"`
}

// llmRunDetailResponse добавляет сырой ответ модели.
type llmRunDetailResponse struct {
	llmRunResponse
	ResponseJSON string `json:"response_json"`
}

// llmUsageResponse — суммарный расход по журналу.
type llmUsageResponse struct {
	Runs         int      `json:"runs"`
	InputTokens  int      `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens"`
	CostEstimate *float64 `json:"cost_estimate"`
	// PricedRuns говорит, по скольким запускам стоимость известна: без этого
	// сумма выглядела бы полной, хотя часть запусков в неё не вошла.
	PricedRuns int `json:"priced_runs"`
}

type llmRunsResponse struct {
	Items []llmRunResponse `json:"items"`
	// Count — сколько записей подходит под фильтр, а не сколько отдано.
	Count int              `json:"count"`
	Usage llmUsageResponse `json:"usage"`
}

func newLLMRunResponse(run model.LLMRun) llmRunResponse {
	resp := llmRunResponse{
		ID:            run.ID,
		CreatedAt:     run.CreatedAt,
		Purpose:       run.Purpose,
		VacancyID:     run.VacancyID,
		Provider:      run.Provider,
		Model:         run.Model,
		PromptVersion: run.PromptVersion,
		SourceURL:     run.SourceURL,
		SourceChars:   run.SourceChars,
		Status:        run.Status,
		InputTokens:   run.InputTokens,
		OutputTokens:  run.OutputTokens,
		CostEstimate:  run.CostEstimate,
		Attempts:      run.Attempts,
		DurationMs:    run.DurationMs,
		Error:         run.Error,
	}

	if run.Vacancy != nil {
		// Должность информативнее компании, но сойдёт и она.
		resp.VacancyTitle = run.Vacancy.Title
		if resp.VacancyTitle == "" {
			resp.VacancyTitle = run.Vacancy.Company
		}
	}
	return resp
}

func (s *Server) handleListLLMRuns(w http.ResponseWriter, r *http.Request) {
	filter := store.LLMRunFilter{}
	errs := fieldErrors{}
	query := r.URL.Query()

	if raw := query.Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 {
			errs.add("limit", "ожидается положительное число")
		}
		filter.Limit = value
	}
	if raw := query.Get("offset"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			errs.add("offset", "ожидается неотрицательное число")
		}
		filter.Offset = value
	}
	if raw := query.Get("vacancy_id"); raw != "" {
		value, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || value == 0 {
			errs.add("vacancy_id", "ожидается идентификатор вакансии")
		} else {
			id := uint(value)
			filter.VacancyID = &id
		}
	}

	if !errs.empty() {
		writeValidationError(w, errs)
		return
	}

	runs, count, err := store.ListLLMRuns(s.db, filter)
	if err != nil {
		s.writeInternalError(w, r, err)
		return
	}

	usage, err := store.LLMRunsUsage(s.db)
	if err != nil {
		s.writeInternalError(w, r, err)
		return
	}

	items := make([]llmRunResponse, 0, len(runs))
	for _, run := range runs {
		items = append(items, newLLMRunResponse(run))
	}

	writeJSON(w, http.StatusOK, llmRunsResponse{
		Items: items,
		Count: count,
		Usage: llmUsageResponse{
			Runs:         usage.Runs,
			InputTokens:  usage.InputTokens,
			OutputTokens: usage.OutputTokens,
			CostEstimate: usage.CostEstimate,
			PricedRuns:   usage.PricedRuns,
		},
	})
}

func (s *Server) handleGetLLMRun(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("id")

	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		writeNotFound(w, "запуск не найден")
		return
	}

	run, err := store.GetLLMRun(s.db, uint(id))
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w, "запуск не найден")
		return
	}
	if err != nil {
		s.writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, llmRunDetailResponse{
		llmRunResponse: newLLMRunResponse(*run),
		ResponseJSON:   run.ResponseJSON,
	})
}
