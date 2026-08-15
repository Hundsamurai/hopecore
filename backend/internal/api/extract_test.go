package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Hundsamurai/hopecore/backend/internal/fetcher"
	"github.com/Hundsamurai/hopecore/backend/internal/llm"
	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

// fullAnswer — ответ модели, в котором заполнено всё.
const fullAnswer = `{
  "title": "Middle Golang разработчик",
  "company": "ПАО Сбербанк",
  "grade": "middle",
  "tech_tags": ["Go", "PostgreSQL", "Kafka"],
  "opened_date": "2026-08-05",
  "salary_from": 300000,
  "salary_to": 450000,
  "salary_currency": "RUB",
  "salary_gross": true,
  "work_format": "onsite"
}`

func (e *testEnv) extract(vacancyID uint, body any) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.request(http.MethodPost, "/api/vacancies/"+itoa(vacancyID)+"/extract", body)
}

func TestExtractReturnsPreviewWithoutWritingVacancy(t *testing.T) {
	env := newTestEnv(t)
	env.enableExtraction(llmSetup{
		provider: &stubProvider{answers: []string{fullAnswer}, tokensIn: 1071, tokensOut: 106},
		fetcher: &stubFetcher{page: fetcher.Page{
			Text:  strings.Repeat("текст вакансии ", 200),
			Chars: 2921,
		}},
	})

	vacancy := env.createVacancy(map[string]any{
		"url":   "https://rabota.sber.ru/vacancy/1",
		"grade": model.GradeJunior,
	})

	var resp extractResponse
	env.decode(env.extract(vacancy.ID, map[string]any{"provider": "gemini"}), http.StatusOK, &resp)

	if resp.RunID == 0 {
		t.Error("run_id не заполнен")
	}
	if resp.SourceChars != 2921 {
		t.Errorf("source_chars = %d", resp.SourceChars)
	}
	if resp.Usage.InputTokens != 1071 || resp.Usage.OutputTokens != 106 {
		t.Errorf("usage = %+v", resp.Usage)
	}

	// Предпросмотр описывает все поля схемы, а не только найденные.
	if len(resp.Fields) != 10 {
		t.Errorf("полей в предпросмотре: %d, ожидалось 10", len(resp.Fields))
	}

	title := resp.Fields["title"]
	if title.Extracted != "Middle Golang разработчик" || !title.HasValue || !title.Differs {
		t.Errorf("title = %+v", title)
	}
	// Грейд отличается от текущего — интерфейс предложит заменить.
	grade := resp.Fields["grade"]
	if grade.Extracted != "middle" || grade.Current != model.GradeJunior || !grade.Differs {
		t.Errorf("grade = %+v", grade)
	}

	// Главное свойство: вакансия не изменилась.
	var stored model.Vacancy
	if err := env.db.First(&stored, vacancy.ID).Error; err != nil {
		t.Fatalf("чтение вакансии: %v", err)
	}
	if stored.Title != "" || stored.Grade != model.GradeJunior {
		t.Errorf("извлечение изменило вакансию: %+v", stored)
	}
}

func TestExtractMarksUnchangedFields(t *testing.T) {
	env := newTestEnv(t)
	env.enableExtraction(llmSetup{provider: &stubProvider{answers: []string{fullAnswer}}})

	// Заводим вакансию с теми же значениями, что вернёт модель.
	vacancy := env.createVacancy(map[string]any{
		"url":         "https://rabota.sber.ru/vacancy/2",
		"title":       "Middle Golang разработчик",
		"company":     "ПАО Сбербанк",
		"grade":       model.GradeMiddle,
		"tech_tags":   []string{"Go", "PostgreSQL", "Kafka"},
		"work_format": model.WorkFormatOnsite,
	})

	var resp extractResponse
	env.decode(env.extract(vacancy.ID, map[string]any{"provider": "gemini"}), http.StatusOK, &resp)

	// Совпадающие поля не предлагаются к замене: галочки ставить не на что.
	for _, name := range []string{"title", "company", "grade", "tech_tags", "work_format"} {
		field := resp.Fields[name]
		if !field.HasValue {
			t.Errorf("%s: has_value = false, хотя модель значение дала", name)
		}
		if field.Differs {
			t.Errorf("%s: differs = true, хотя значение совпадает с текущим (%v против %v)",
				name, field.Extracted, field.Current)
		}
	}

	// А отсутствующие в карточке — предлагаются.
	if !resp.Fields["salary_from"].Differs {
		t.Error("salary_from: differs = false, хотя в карточке вилки нет")
	}
}

func TestExtractKeepsGoodFieldsWhenSomeRejected(t *testing.T) {
	env := newTestEnv(t)
	// Грейд не из набора, дата в неверном формате — остальное годное.
	answer := `{"title":"Go-разработчик","grade":"боженька","opened_date":"05.08.2026","work_format":"remote"}`
	env.enableExtraction(llmSetup{provider: &stubProvider{answers: []string{answer}}})

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/1"})

	var resp extractResponse
	env.decode(env.extract(vacancy.ID, map[string]any{"provider": "gemini"}), http.StatusOK, &resp)

	// Годное осталось.
	if !resp.Fields["title"].HasValue || !resp.Fields["work_format"].HasValue {
		t.Errorf("годные поля потерялись: %+v", resp.Fields)
	}

	// Негодное не предлагается, но объяснено.
	for _, name := range []string{"grade", "opened_date"} {
		field := resp.Fields[name]
		if field.HasValue {
			t.Errorf("%s: has_value = true, хотя значение отброшено", name)
		}
		if field.Extracted != nil {
			t.Errorf("%s: extracted = %v, ожидался null", name, field.Extracted)
		}
		if field.Note == "" {
			t.Errorf("%s: нет объяснения, почему значение отброшено", name)
		}
		if field.Differs {
			t.Errorf("%s: differs = true у отброшенного значения", name)
		}
	}
}

func TestExtractEmptyAnswerIsValid(t *testing.T) {
	env := newTestEnv(t)
	// Модель ничего не нашла — законный результат, не ошибка.
	answer := `{"title":"","company":"","grade":null,"tech_tags":[],"opened_date":null,
	"salary_from":null,"salary_to":null,"salary_currency":null,"salary_gross":null,"work_format":null}`
	env.enableExtraction(llmSetup{provider: &stubProvider{answers: []string{answer}}})

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/empty"})

	var resp extractResponse
	env.decode(env.extract(vacancy.ID, map[string]any{"provider": "gemini"}), http.StatusOK, &resp)

	for name, field := range resp.Fields {
		if field.HasValue {
			t.Errorf("%s: has_value = true при пустом ответе", name)
		}
	}
}

func TestExtractPassesPageWarnings(t *testing.T) {
	env := newTestEnv(t)
	env.enableExtraction(llmSetup{
		provider: &stubProvider{answers: []string{`{"title":"Go-разработчик"}`}},
		fetcher: &stubFetcher{page: fetcher.Page{
			Text:     "мало текста",
			Chars:    11,
			Warnings: []string{"со страницы удалось прочитать только 11 символов текста"},
		}},
	})

	vacancy := env.createVacancy(map[string]any{"url": "https://tbank.ru/career/1"})

	var resp extractResponse
	env.decode(env.extract(vacancy.ID, map[string]any{"provider": "gemini"}), http.StatusOK, &resp)

	// Пользователь должен понимать, почему результат скудный.
	if len(resp.Warnings) == 0 {
		t.Error("предупреждения о странице не доехали до клиента")
	}
}

func TestExtractCostEstimate(t *testing.T) {
	t.Run("с прайсом", func(t *testing.T) {
		env := newTestEnv(t)
		env.enableExtraction(llmSetup{
			provider: &stubProvider{answers: []string{fullAnswer}, tokensIn: 1_000_000, tokensOut: 1_000_000},
			price:    llm.Pricing{InputPerMillion: 0.3, OutputPerMillion: 2.5},
		})

		vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/priced"})

		var resp extractResponse
		env.decode(env.extract(vacancy.ID, map[string]any{"provider": "gemini"}), http.StatusOK, &resp)

		if resp.Usage.CostEstimate == nil {
			t.Fatal("cost_estimate = null, хотя прайс задан")
		}
		if diff := *resp.Usage.CostEstimate - 2.8; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("cost_estimate = %v, ожидалось 2.8", *resp.Usage.CostEstimate)
		}
	})

	t.Run("без прайса", func(t *testing.T) {
		env := newTestEnv(t)
		env.enableExtraction(llmSetup{
			provider: &stubProvider{answers: []string{fullAnswer}, tokensIn: 1000, tokensOut: 100},
		})

		vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/free"})

		var resp extractResponse
		env.decode(env.extract(vacancy.ID, map[string]any{"provider": "gemini"}), http.StatusOK, &resp)

		// Без прайса показываем только токены: выдуманная сумма хуже отсутствия.
		if resp.Usage.CostEstimate != nil {
			t.Errorf("cost_estimate = %v, ожидался null", *resp.Usage.CostEstimate)
		}
		if resp.Usage.InputTokens != 1000 {
			t.Errorf("input_tokens = %d", resp.Usage.InputTokens)
		}
	})
}

func TestExtractWritesRunToJournal(t *testing.T) {
	env := newTestEnv(t)
	env.enableExtraction(llmSetup{
		provider: &stubProvider{answers: []string{fullAnswer}, tokensIn: 1071, tokensOut: 106},
	})

	vacancy := env.createVacancy(map[string]any{"url": "https://rabota.sber.ru/vacancy/3"})
	env.decode(env.extract(vacancy.ID, map[string]any{"provider": "gemini", "model": "gemini-2.5-flash"}), http.StatusOK, nil)

	var runs llmRunsResponse
	env.decode(env.request(http.MethodGet, "/api/llm/runs", nil), http.StatusOK, &runs)

	if runs.Count != 1 {
		t.Fatalf("записей в журнале: %d", runs.Count)
	}
	run := runs.Items[0]
	if run.Status != model.RunStatusOK {
		t.Errorf("status = %q", run.Status)
	}
	if run.Provider != "gemini" || run.Model != "gemini-2.5-flash" {
		t.Errorf("провайдер и модель: %+v", run)
	}
	if run.PromptVersion == "" {
		t.Error("версия промпта не записана: без неё нельзя объяснить старый результат")
	}
	if run.VacancyID == nil || *run.VacancyID != vacancy.ID {
		t.Errorf("vacancy_id = %v", run.VacancyID)
	}
	if run.Attempts != 1 {
		t.Errorf("attempts = %d", run.Attempts)
	}

	// Сырой ответ доступен в карточке запуска.
	var detail llmRunDetailResponse
	env.decode(env.request(http.MethodGet, "/api/llm/runs/"+itoa(run.ID), nil), http.StatusOK, &detail)
	if !strings.Contains(detail.ResponseJSON, "Golang") {
		t.Errorf("сырой ответ не сохранён: %q", detail.ResponseJSON)
	}
}

func TestExtractRetriesOnUnparsableAnswer(t *testing.T) {
	env := newTestEnv(t)
	// Первый ответ — без JSON, второй нормальный.
	env.enableExtraction(llmSetup{provider: &stubProvider{
		answers: []string{"Извините, не могу обработать страницу", fullAnswer},
	}})

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/retry"})

	var resp extractResponse
	env.decode(env.extract(vacancy.ID, map[string]any{"provider": "gemini"}), http.StatusOK, &resp)

	if resp.Usage.Attempts != 2 {
		t.Errorf("attempts = %d, ожидалось 2", resp.Usage.Attempts)
	}
	if !resp.Fields["title"].HasValue {
		t.Error("вторая попытка не дала результата")
	}

	// Во второй запрос должно уйти объяснение, что было не так.
	if len(env.provider.requests) != 2 {
		t.Fatalf("запросов к модели: %d", len(env.provider.requests))
	}
	if !strings.Contains(env.provider.requests[1].User, "не подошёл") {
		t.Error("во второй попытке нет объяснения проблемы")
	}
}

func TestExtractFailsAfterTwoUnparsableAnswers(t *testing.T) {
	env := newTestEnv(t)
	env.enableExtraction(llmSetup{provider: &stubProvider{
		answers: []string{"не json", "снова не json"},
	}})

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/bad"})

	payload := env.decodeError(
		env.extract(vacancy.ID, map[string]any{"provider": "gemini"}),
		http.StatusBadGateway,
	)
	if payload.Code != CodeExtractionFailed {
		t.Errorf("code = %q, ожидалось %q", payload.Code, CodeExtractionFailed)
	}

	// Неудача тоже попадает в журнал, вместе с потраченными токенами.
	var runs llmRunsResponse
	env.decode(env.request(http.MethodGet, "/api/llm/runs", nil), http.StatusOK, &runs)
	if runs.Count != 1 {
		t.Fatalf("записей в журнале: %d", runs.Count)
	}
	if runs.Items[0].Status != model.RunStatusInvalidJSON {
		t.Errorf("status = %q, ожидался %q", runs.Items[0].Status, model.RunStatusInvalidJSON)
	}
	if runs.Items[0].Attempts != 2 {
		t.Errorf("attempts = %d, ожидалось 2", runs.Items[0].Attempts)
	}
}

func TestExtractFetchErrorRecorded(t *testing.T) {
	env := newTestEnv(t)
	env.enableExtraction(llmSetup{
		fetcher: &stubFetcher{err: &fetcher.Error{
			Kind:       fetcher.KindStatus,
			StatusCode: http.StatusForbidden,
			Message:    "сайт ответил 403, страницу прочитать не удалось",
		}},
	})

	vacancy := env.createVacancy(map[string]any{"url": "https://ozon.tech/vacancies/1"})

	payload := env.decodeError(
		env.extract(vacancy.ID, map[string]any{"provider": "gemini"}),
		http.StatusBadGateway,
	)
	if !strings.Contains(payload.Message, "403") {
		t.Errorf("сообщение = %q, ожидалось упоминание кода", payload.Message)
	}

	var runs llmRunsResponse
	env.decode(env.request(http.MethodGet, "/api/llm/runs", nil), http.StatusOK, &runs)
	if runs.Count != 1 {
		t.Fatalf("записей в журнале: %d", runs.Count)
	}
	run := runs.Items[0]
	if run.Status != model.RunStatusFetchError {
		t.Errorf("status = %q", run.Status)
	}
	// Токены не тратились: до модели дело не дошло.
	if run.InputTokens != nil {
		t.Errorf("input_tokens = %v, ожидался null", *run.InputTokens)
	}
	if len(env.provider.requests) != 0 {
		t.Error("модель вызывалась, хотя страница не скачалась")
	}
}

func TestExtractTimeoutRecordedSeparately(t *testing.T) {
	env := newTestEnv(t)
	env.enableExtraction(llmSetup{
		fetcher: &stubFetcher{err: &fetcher.Error{
			Kind:    fetcher.KindTimeout,
			Message: "таймаут скачивания страницы",
		}},
	})

	vacancy := env.createVacancy(map[string]any{"url": "https://slow.example.com/vacancy"})
	env.decodeError(env.extract(vacancy.ID, map[string]any{"provider": "gemini"}), http.StatusBadGateway)

	var runs llmRunsResponse
	env.decode(env.request(http.MethodGet, "/api/llm/runs", nil), http.StatusOK, &runs)
	// Таймаут и отказ сайта — разные истории, в журнале это видно.
	if runs.Items[0].Status != model.RunStatusTimeout {
		t.Errorf("status = %q, ожидался %q", runs.Items[0].Status, model.RunStatusTimeout)
	}
}

func TestExtractProviderErrorRecorded(t *testing.T) {
	env := newTestEnv(t)
	env.enableExtraction(llmSetup{provider: &stubProvider{
		errs:     []error{errors.New("Gemini: превышена квота или частота запросов")},
		tokensIn: 500,
	}})

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/quota"})

	payload := env.decodeError(
		env.extract(vacancy.ID, map[string]any{"provider": "gemini"}),
		http.StatusBadGateway,
	)
	if !strings.Contains(payload.Message, "квота") {
		t.Errorf("сообщение = %q", payload.Message)
	}

	var runs llmRunsResponse
	env.decode(env.request(http.MethodGet, "/api/llm/runs", nil), http.StatusOK, &runs)
	run := runs.Items[0]
	if run.Status != model.RunStatusProviderError {
		t.Errorf("status = %q", run.Status)
	}
	// Ошибка провайдера повторной попытки не заслуживает: квота сама не пройдёт.
	if run.Attempts != 1 {
		t.Errorf("attempts = %d, ожидалось 1", run.Attempts)
	}
	// Токены, если провайдер их посчитал, всё равно записаны.
	if run.InputTokens == nil || *run.InputTokens != 500 {
		t.Errorf("input_tokens = %v, ожидалось 500", run.InputTokens)
	}
}

func TestExtractWithoutProvidersConfigured(t *testing.T) {
	env := newTestEnv(t)
	// Извлечение не подключено: ключей нет.
	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/no-keys"})

	payload := env.decodeError(
		env.extract(vacancy.ID, map[string]any{"provider": "gemini"}),
		http.StatusBadRequest,
	)
	if payload.Code != CodeProviderUnavailable {
		t.Errorf("code = %q, ожидалось %q", payload.Code, CodeProviderUnavailable)
	}
	if !strings.Contains(payload.Message, ".env") {
		t.Errorf("сообщение не подсказывает, что делать: %q", payload.Message)
	}
}

func TestExtractValidation(t *testing.T) {
	env := newTestEnv(t)
	env.enableExtraction(llmSetup{provider: &stubProvider{answers: []string{fullAnswer}}})

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/validate"})

	t.Run("провайдер не указан", func(t *testing.T) {
		payload := env.decodeError(env.extract(vacancy.ID, map[string]any{}), http.StatusBadRequest)
		if _, ok := payload.Fields["provider"]; !ok {
			t.Errorf("в fields нет provider: %v", payload.Fields)
		}
	})

	t.Run("неизвестный провайдер", func(t *testing.T) {
		payload := env.decodeError(
			env.extract(vacancy.ID, map[string]any{"provider": "openai"}),
			http.StatusBadRequest,
		)
		if payload.Code != CodeProviderUnavailable {
			t.Errorf("code = %q", payload.Code)
		}
	})

	t.Run("модель вне списка", func(t *testing.T) {
		// Иначе клиент мог бы потратить квоту на произвольную модель.
		payload := env.decodeError(
			env.extract(vacancy.ID, map[string]any{"provider": "gemini", "model": "gemini-3.1-pro-preview"}),
			http.StatusBadRequest,
		)
		if _, ok := payload.Fields["model"]; !ok {
			t.Errorf("в fields нет model: %v", payload.Fields)
		}
	})

	t.Run("неизвестное поле в теле", func(t *testing.T) {
		payload := env.decodeError(
			env.extract(vacancy.ID, `{"provider":"gemini","temperature":0.9}`),
			http.StatusBadRequest,
		)
		if payload.Code != CodeInvalidJSON {
			t.Errorf("code = %q", payload.Code)
		}
	})

	t.Run("несуществующая вакансия", func(t *testing.T) {
		payload := env.decodeError(
			env.request(http.MethodPost, "/api/vacancies/999/extract", map[string]any{"provider": "gemini"}),
			http.StatusNotFound,
		)
		if payload.Code != CodeNotFound {
			t.Errorf("code = %q", payload.Code)
		}
	})
}

func TestExtractUsesDefaultModel(t *testing.T) {
	env := newTestEnv(t)
	env.enableExtraction(llmSetup{
		provider: &stubProvider{answers: []string{fullAnswer}},
		models:   []string{"gemini-2.5-flash", "gemini-flash-latest"},
	})

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/default-model"})

	var resp extractResponse
	env.decode(env.extract(vacancy.ID, map[string]any{"provider": "gemini"}), http.StatusOK, &resp)

	// Без указанной модели берётся первая из списка провайдера.
	if resp.Model != "gemini-2.5-flash" {
		t.Errorf("model = %q, ожидалась первая из списка", resp.Model)
	}
	if env.provider.requests[0].Model != "gemini-2.5-flash" {
		t.Errorf("в запросе модель = %q", env.provider.requests[0].Model)
	}
}

func TestExtractSendsPageTextAndSchema(t *testing.T) {
	env := newTestEnv(t)
	env.enableExtraction(llmSetup{
		provider: &stubProvider{answers: []string{fullAnswer}},
		fetcher: &stubFetcher{page: fetcher.Page{
			Text:  "Заголовок страницы: Go-разработчик в ООО Пример",
			Chars: 46,
		}},
	})

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/prompt"})
	env.decode(env.extract(vacancy.ID, map[string]any{"provider": "gemini"}), http.StatusOK, nil)

	request := env.provider.requests[0]
	if !strings.Contains(request.User, "ООО Пример") {
		t.Error("текст страницы не попал в запрос")
	}
	if !strings.Contains(request.User, vacancy.URL) {
		t.Error("ссылка не попала в запрос")
	}
	if !strings.Contains(request.System, "Не выдумывай") {
		t.Error("системный промпт не передан")
	}
	if len(request.Schema.Fields) != 10 {
		t.Errorf("полей в схеме: %d", len(request.Schema.Fields))
	}

	// Фетчер получил ссылку вакансии, а не что-то ещё.
	if len(env.fetcher.calls) != 1 || env.fetcher.calls[0] != vacancy.URL {
		t.Errorf("фетчер вызван с %v", env.fetcher.calls)
	}
}

func TestExtractPreviewCoversAllSchemaFields(t *testing.T) {
	env := newTestEnv(t)
	env.enableExtraction(llmSetup{provider: &stubProvider{answers: []string{fullAnswer}}})

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/fields"})

	var resp extractResponse
	env.decode(env.extract(vacancy.ID, map[string]any{"provider": "gemini"}), http.StatusOK, &resp)

	// Набор полей предпросмотра должен совпадать со схемой извлечения:
	// разъехавшись, они дадут молча пропавшее поле в интерфейсе.
	for _, name := range llm.ExtractionSchema().FieldNames() {
		if _, ok := resp.Fields[name]; !ok {
			t.Errorf("поле %q есть в схеме, но не пришло в предпросмотре", name)
		}
	}
}

func TestExtractRejectsGet(t *testing.T) {
	env := newTestEnv(t)
	env.enableExtraction(llmSetup{})

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/method"})

	rec := env.request(http.MethodGet, "/api/vacancies/"+itoa(vacancy.ID)+"/extract", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("статус = %d, ожидался %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
