package api

import (
	"errors"
	"net/http"
	"reflect"

	"github.com/Hundsamurai/hopecore/backend/internal/llm"
	"github.com/Hundsamurai/hopecore/backend/internal/model"
	"github.com/Hundsamurai/hopecore/backend/internal/service"
	"github.com/Hundsamurai/hopecore/backend/internal/store"
)

// extractRequest — тело POST /api/vacancies/{id}/extract.
// Модель необязательна: без неё берётся первая из списка провайдера.
type extractRequest struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// extractedFieldResponse — одно поле в предпросмотре.
//
// Extracted и Current намеренно нетипизированы: поля разного вида — строки,
// числа, массив тегов, дата, признак «до вычета налогов».
type extractedFieldResponse struct {
	Extracted any `json:"extracted"`
	Current   any `json:"current"`
	// HasValue — модель дала пригодное значение. Если false, а Note не пуст,
	// значение было и его отбросили при проверке.
	HasValue bool `json:"has_value"`
	// Differs — предложенное отличается от того, что уже в карточке.
	// Интерфейс отмечает галочками именно такие поля.
	Differs bool   `json:"differs"`
	Note    string `json:"note,omitempty"`
}

type extractResponse struct {
	RunID    uint   `json:"run_id"`
	Provider string `json:"provider"`
	Model    string `json:"model"`

	SourceURL   string `json:"source_url"`
	SourceChars int    `json:"source_chars"`
	// Warnings — про страницу целиком: мало текста, антибот, обрезка.
	Warnings []string `json:"warnings"`

	Fields map[string]extractedFieldResponse `json:"fields"`
	Usage  extractUsageResponse              `json:"usage"`
}

type extractUsageResponse struct {
	InputTokens  int      `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens"`
	CostEstimate *float64 `json:"cost_estimate"`
	Attempts     int      `json:"attempts"`
	DurationMs   int      `json:"duration_ms"`
}

func (s *Server) handleExtractVacancy(w http.ResponseWriter, r *http.Request) {
	id, ok := s.vacancyID(w, r)
	if !ok {
		return
	}

	var req extractRequest
	if r.ContentLength > 0 {
		if !s.decodeJSON(w, r, &req) {
			return
		}
	}

	if s.extraction == nil || !s.extraction.Enabled() {
		writeError(w, http.StatusBadRequest, CodeProviderUnavailable,
			"провайдеры языковых моделей не настроены: добавьте ключ в .env")
		return
	}
	if req.Provider == "" {
		errs := fieldErrors{}
		errs.add("provider", "обязательное поле: выберите провайдера из /api/llm/providers")
		writeValidationError(w, errs)
		return
	}

	extraction, err := s.extraction.Extract(r.Context(), id, req.Provider, req.Model)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeNotFound(w, "вакансия не найдена")
		return
	case errors.Is(err, service.ErrProviderUnavailable):
		writeError(w, http.StatusBadRequest, CodeProviderUnavailable, err.Error())
		return
	case errors.Is(err, service.ErrModelNotAllowed):
		errs := fieldErrors{}
		errs.add("model", err.Error())
		writeValidationError(w, errs)
		return
	case errors.Is(err, service.ErrExtractionFailed):
		// Не ошибка сервера: страница могла быть закрыта антиботом, а модель —
		// упереться в квоту. Подробности уже в журнале запусков.
		writeError(w, http.StatusBadGateway, CodeExtractionFailed, err.Error())
		return
	case err != nil:
		s.writeInternalError(w, r, err)
		return
	}

	vacancy, err := store.GetVacancy(s.db, id)
	if err != nil {
		s.writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, newExtractResponse(extraction, *vacancy))
}

func newExtractResponse(extraction service.Extraction, vacancy model.Vacancy) extractResponse {
	return extractResponse{
		RunID:       extraction.RunID,
		Provider:    extraction.Provider,
		Model:       extraction.Model,
		SourceURL:   extraction.SourceURL,
		SourceChars: extraction.SourceChars,
		Warnings:    orEmpty(extraction.Warnings),
		Fields:      buildFields(extraction, vacancy),
		Usage: extractUsageResponse{
			InputTokens:  extraction.InputTokens,
			OutputTokens: extraction.OutputTokens,
			CostEstimate: extraction.CostEstimate,
			Attempts:     extraction.Attempts,
			DurationMs:   extraction.DurationMs,
		},
	}
}

// buildFields собирает предпросмотр по всем полям схемы.
//
// Возвращаются все поля, а не только найденные: интерфейсу нужно показать
// полную таблицу «сейчас против предложенного», включая строки, где модель
// ничего не нашла или где её значение отбросили.
func buildFields(extraction service.Extraction, vacancy model.Vacancy) map[string]extractedFieldResponse {
	notes := map[string]string{}
	for _, note := range extraction.Notes {
		if _, exists := notes[note.Field]; !exists {
			notes[note.Field] = note.Note
		}
	}

	values := extraction.Values
	fields := map[string]extractedFieldResponse{
		"title":           {Extracted: emptyToNil(values.Title), Current: emptyToNil(vacancy.Title)},
		"company":         {Extracted: emptyToNil(values.Company), Current: emptyToNil(vacancy.Company)},
		"grade":           {Extracted: emptyToNil(values.Grade), Current: emptyToNil(vacancy.Grade)},
		"tech_tags":       {Extracted: tagsOrNil(values.TechTags), Current: tagsOrNil(vacancy.TechTags)},
		"opened_date":     {Extracted: dateOrNil(values.OpenedDate), Current: dateOrNil(vacancy.OpenedDate)},
		"salary_from":     {Extracted: floatOrNil(values.SalaryFrom), Current: floatOrNil(vacancy.SalaryFrom)},
		"salary_to":       {Extracted: floatOrNil(values.SalaryTo), Current: floatOrNil(vacancy.SalaryTo)},
		"salary_currency": {Extracted: emptyToNil(values.SalaryCurrency), Current: emptyToNil(vacancy.SalaryCurrency)},
		"salary_gross":    {Extracted: boolOrNil(values.SalaryGross), Current: boolOrNil(vacancy.SalaryGross)},
		"work_format":     {Extracted: emptyToNil(values.WorkFormat), Current: emptyToNil(vacancy.WorkFormat)},
	}

	for name, field := range fields {
		field.HasValue = extraction.Filled[name]
		field.Note = notes[name]
		// Отброшенное значение не предлагается к применению, даже если оно
		// отличается от текущего.
		field.Differs = field.HasValue && !reflect.DeepEqual(field.Extracted, field.Current)

		if !field.HasValue {
			// Чтобы интерфейс не показал «модель предлагает пусто» там,
			// где она просто ничего не нашла.
			field.Extracted = nil
		}
		fields[name] = field
	}

	return fields
}

func emptyToNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func tagsOrNil(tags []string) any {
	if len(tags) == 0 {
		return nil
	}
	return []string(tags)
}

func dateOrNil(date *model.Date) any {
	if date == nil {
		return nil
	}
	return date.String()
}

func floatOrNil(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func boolOrNil(value *bool) any {
	if value == nil {
		return nil
	}
	return *value
}

func orEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

// Проверка на этапе компиляции, что схема извлечения и предпросмотр
// описывают один и тот же набор полей: разъехавшись, они дадут молча
// пропавшее поле в интерфейсе.
var _ = func() struct{} {
	preview := buildFields(service.Extraction{Filled: map[string]bool{}}, model.Vacancy{})
	for _, name := range llm.ExtractionSchema().FieldNames() {
		if _, ok := preview[name]; !ok {
			panic("поле " + name + " есть в схеме извлечения, но не попадает в предпросмотр")
		}
	}
	if len(preview) != len(llm.ExtractionSchema().Fields) {
		panic("число полей в предпросмотре не совпадает со схемой извлечения")
	}
	return struct{}{}
}()
