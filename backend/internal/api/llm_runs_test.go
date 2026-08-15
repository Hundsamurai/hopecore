package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

// seedRun пишет запись журнала напрямую: эндпоинта записи нет и не будет,
// журнал заполняет сервис извлечения.
func (e *testEnv) seedRun(run model.LLMRun) model.LLMRun {
	e.t.Helper()

	if run.Purpose == "" {
		run.Purpose = model.PurposeExtractVacancy
	}
	if err := e.db.Create(&run).Error; err != nil {
		e.t.Fatalf("создание записи журнала: %v", err)
	}
	return run
}

func intPtrValue(v int) *int { return &v }

func TestListLLMRunsEmpty(t *testing.T) {
	env := newTestEnv(t)

	var resp llmRunsResponse
	env.decode(env.request(http.MethodGet, "/api/llm/runs", nil), http.StatusOK, &resp)

	if resp.Items == nil {
		t.Error("items = null, ожидался пустой массив")
	}
	if resp.Count != 0 || resp.Usage.Runs != 0 {
		t.Errorf("пустой журнал дал count=%d usage=%+v", resp.Count, resp.Usage)
	}
	// Без запусков с прайсом суммы нет: ноль был бы обманом.
	if resp.Usage.CostEstimate != nil {
		t.Errorf("cost_estimate = %v, ожидался null", *resp.Usage.CostEstimate)
	}
}

func TestListLLMRunsReturnsUsageAndTitle(t *testing.T) {
	env := newTestEnv(t)

	vacancy := env.createVacancy(map[string]any{
		"url":   "https://example.com/jobs/1",
		"title": "Go-разработчик",
	})

	cost := 0.0032
	env.seedRun(model.LLMRun{
		VacancyID:     &vacancy.ID,
		Provider:      "gemini",
		Model:         "gemini-2.5-flash",
		PromptVersion: "extract-v1",
		SourceURL:     vacancy.URL,
		SourceChars:   2921,
		Status:        model.RunStatusOK,
		InputTokens:   intPtrValue(1071),
		OutputTokens:  intPtrValue(106),
		CostEstimate:  &cost,
		Attempts:      1,
		DurationMs:    2170,
		ResponseJSON:  `{"title":"Go-разработчик"}`,
	})

	var resp llmRunsResponse
	env.decode(env.request(http.MethodGet, "/api/llm/runs", nil), http.StatusOK, &resp)

	if len(resp.Items) != 1 || resp.Count != 1 {
		t.Fatalf("items = %d, count = %d", len(resp.Items), resp.Count)
	}

	item := resp.Items[0]
	if item.Provider != "gemini" || item.Model != "gemini-2.5-flash" {
		t.Errorf("провайдер и модель: %+v", item)
	}
	// Экрану журнала нужна подпись, а не только идентификатор.
	if item.VacancyTitle != "Go-разработчик" {
		t.Errorf("vacancy_title = %q", item.VacancyTitle)
	}
	if item.SourceChars != 2921 || item.DurationMs != 2170 {
		t.Errorf("диагностика запуска: %+v", item)
	}
	if item.InputTokens == nil || *item.InputTokens != 1071 {
		t.Errorf("input_tokens = %v", item.InputTokens)
	}

	if resp.Usage.InputTokens != 1071 || resp.Usage.OutputTokens != 106 {
		t.Errorf("usage = %+v", resp.Usage)
	}
	if resp.Usage.CostEstimate == nil || resp.Usage.PricedRuns != 1 {
		t.Errorf("оценка стоимости не собрана: %+v", resp.Usage)
	}
}

func TestListLLMRunsOmitsRawResponse(t *testing.T) {
	env := newTestEnv(t)

	env.seedRun(model.LLMRun{
		Provider:     "gemini",
		Model:        "gemini-2.5-flash",
		Status:       model.RunStatusOK,
		ResponseJSON: `{"секрет":"этого не должно быть в списке"}`,
	})

	rec := env.request(http.MethodGet, "/api/llm/runs", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("статус = %d", rec.Code)
	}

	// Сырой ответ отдаётся только в карточке одного запуска.
	body := rec.Body.String()
	if strings.Contains(body, "этого не должно быть") || strings.Contains(body, "response_json") {
		t.Errorf("в списке пришёл сырой ответ: %s", body)
	}
}

func TestGetLLMRunReturnsRawResponse(t *testing.T) {
	env := newTestEnv(t)

	run := env.seedRun(model.LLMRun{
		Provider:     "gemini",
		Model:        "gemini-2.5-flash",
		Status:       model.RunStatusOK,
		ResponseJSON: `{"title":"Go-разработчик","salary_from":null}`,
	})

	var resp llmRunDetailResponse
	env.decode(env.request(http.MethodGet, "/api/llm/runs/"+itoa(run.ID), nil), http.StatusOK, &resp)

	if resp.ID != run.ID {
		t.Errorf("id = %d, ожидался %d", resp.ID, run.ID)
	}
	// Возможность посмотреть, что именно сказала модель, — смысл этой карточки.
	if resp.ResponseJSON != `{"title":"Go-разработчик","salary_from":null}` {
		t.Errorf("response_json = %q", resp.ResponseJSON)
	}
}

func TestGetLLMRunNotFound(t *testing.T) {
	env := newTestEnv(t)

	for _, path := range []string{"/api/llm/runs/999", "/api/llm/runs/0", "/api/llm/runs/abc"} {
		t.Run(path, func(t *testing.T) {
			payload := env.decodeError(env.request(http.MethodGet, path, nil), http.StatusNotFound)
			if payload.Code != CodeNotFound {
				t.Errorf("code = %q", payload.Code)
			}
		})
	}
}

func TestListLLMRunsFilterByVacancy(t *testing.T) {
	env := newTestEnv(t)

	first := env.createVacancy(map[string]any{"url": "https://example.com/jobs/first"})
	second := env.createVacancy(map[string]any{"url": "https://example.com/jobs/second"})

	for _, id := range []uint{first.ID, first.ID, second.ID} {
		vacancyID := id
		env.seedRun(model.LLMRun{
			VacancyID: &vacancyID,
			Provider:  "gemini",
			Model:     "gemini-2.5-flash",
			Status:    model.RunStatusOK,
		})
	}

	var resp llmRunsResponse
	env.decode(
		env.request(http.MethodGet, "/api/llm/runs?vacancy_id="+itoa(first.ID), nil),
		http.StatusOK, &resp,
	)

	if resp.Count != 2 {
		t.Errorf("count = %d, ожидалось 2", resp.Count)
	}
	for _, item := range resp.Items {
		if item.VacancyID == nil || *item.VacancyID != first.ID {
			t.Errorf("в выборке чужой запуск: %v", item.VacancyID)
		}
	}

	// Итоги считаются по всему журналу, а не по фильтру: это расход по квоте.
	if resp.Usage.Runs != 3 {
		t.Errorf("usage.runs = %d, ожидалось 3 (итоги по всему журналу)", resp.Usage.Runs)
	}
}

func TestListLLMRunsRecordsSurviveVacancyDeletion(t *testing.T) {
	env := newTestEnv(t)

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/doomed", "title": "Go"})
	vacancyID := vacancy.ID
	env.seedRun(model.LLMRun{
		VacancyID:    &vacancyID,
		Provider:     "gemini",
		Model:        "gemini-2.5-flash",
		Status:       model.RunStatusOK,
		InputTokens:  intPtrValue(1000),
		OutputTokens: intPtrValue(100),
	})

	rec := env.request(http.MethodDelete, "/api/vacancies/"+itoa(vacancy.ID), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("удаление вакансии: статус %d", rec.Code)
	}

	var resp llmRunsResponse
	env.decode(env.request(http.MethodGet, "/api/llm/runs", nil), http.StatusOK, &resp)

	// История трат переживает карточку: запись остаётся, ссылка обнуляется.
	if resp.Count != 1 {
		t.Fatalf("count = %d, ожидалась 1 запись", resp.Count)
	}
	if resp.Items[0].VacancyID != nil {
		t.Errorf("vacancy_id = %v, ожидался null", *resp.Items[0].VacancyID)
	}
	if resp.Items[0].VacancyTitle != "" {
		t.Errorf("vacancy_title = %q, ожидалась пустая строка", resp.Items[0].VacancyTitle)
	}
	if resp.Usage.InputTokens != 1000 {
		t.Errorf("расход потерялся: %+v", resp.Usage)
	}
}

func TestListLLMRunsPagination(t *testing.T) {
	env := newTestEnv(t)

	for i := 0; i < 5; i++ {
		env.seedRun(model.LLMRun{
			Provider: "gemini",
			Model:    "gemini-2.5-flash",
			Status:   model.RunStatusOK,
		})
	}

	var resp llmRunsResponse
	env.decode(env.request(http.MethodGet, "/api/llm/runs?limit=2&offset=1", nil), http.StatusOK, &resp)

	if len(resp.Items) != 2 {
		t.Errorf("отдано %d записей, ожидалось 2", len(resp.Items))
	}
	if resp.Count != 5 {
		t.Errorf("count = %d, ожидалось 5: это число подходящих, а не отданных", resp.Count)
	}
}

func TestListLLMRunsQueryValidation(t *testing.T) {
	env := newTestEnv(t)

	cases := []struct {
		query     string
		wantField string
	}{
		{query: "?limit=0", wantField: "limit"},
		{query: "?limit=много", wantField: "limit"},
		{query: "?offset=-1", wantField: "offset"},
		{query: "?vacancy_id=0", wantField: "vacancy_id"},
		{query: "?vacancy_id=abc", wantField: "vacancy_id"},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			payload := env.decodeError(
				env.request(http.MethodGet, "/api/llm/runs"+tc.query, nil),
				http.StatusBadRequest,
			)
			if _, ok := payload.Fields[tc.wantField]; !ok {
				t.Errorf("в fields нет %q: %v", tc.wantField, payload.Fields)
			}
		})
	}
}

func TestListLLMRunsCapsLimit(t *testing.T) {
	env := newTestEnv(t)

	// Слишком большой лимит не ошибка, он просто урезается.
	rec := env.request(http.MethodGet, "/api/llm/runs?limit=100000", nil)
	if rec.Code != http.StatusOK {
		t.Errorf("статус = %d, ожидался 200: лимит должен урезаться, а не отклоняться", rec.Code)
	}
}

func TestListLLMRunsKeepsFailedRuns(t *testing.T) {
	env := newTestEnv(t)

	// Ошибка скачивания тоже попадает в журнал: видно, что попытка была
	// и почему она ничего не стоила.
	env.seedRun(model.LLMRun{
		Provider:  "gemini",
		Model:     "gemini-2.5-flash",
		SourceURL: "https://ozon.tech/vacancies/1",
		Status:    model.RunStatusFetchError,
		Error:     "сайт ответил 403, страницу прочитать не удалось",
	})

	var resp llmRunsResponse
	env.decode(env.request(http.MethodGet, "/api/llm/runs", nil), http.StatusOK, &resp)

	if len(resp.Items) != 1 {
		t.Fatalf("записей: %d", len(resp.Items))
	}
	item := resp.Items[0]
	if item.Status != model.RunStatusFetchError {
		t.Errorf("status = %q", item.Status)
	}
	if item.Error == "" {
		t.Error("причина неудачи не сохранена")
	}
	if item.InputTokens != nil {
		t.Errorf("input_tokens = %v, ожидался null: токены не тратились", *item.InputTokens)
	}
}

func TestLLMRunsRejectPost(t *testing.T) {
	env := newTestEnv(t)

	// Журнал пишет сервис, а не клиент.
	rec := env.request(http.MethodPost, "/api/llm/runs", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("статус = %d, ожидался %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
