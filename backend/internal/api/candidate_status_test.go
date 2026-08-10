package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

func fullStatusBody() map[string]any {
	return map[string]any{
		"cover_letter":         "Здравствуйте, откликаюсь на вакансию.",
		"sent_at":              "2026-07-15",
		"interview_stage":      "техническое интервью",
		"hr_contact":           "@hr_example",
		"interview_record_url": "https://example.com/record/1",
		"offer_received":       false,
		"offered_salary":       350000,
		"real_salary":          320000.5,
		"market_salary_data":   "медиана по рынку 300k",
	}
}

func TestPutCandidateStatusCreates(t *testing.T) {
	env := newTestEnv(t)

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/1"})

	var status candidateStatusResponse
	env.decode(
		env.request(http.MethodPut, "/api/vacancies/"+itoa(vacancy.ID)+"/candidate-status", fullStatusBody()),
		http.StatusOK, &status,
	)

	if status.ID == 0 {
		t.Fatal("id не присвоен")
	}
	if status.VacancyID != vacancy.ID {
		t.Errorf("vacancy_id = %d, ожидалось %d", status.VacancyID, vacancy.ID)
	}
	if status.InterviewStage != "техническое интервью" {
		t.Errorf("interview_stage = %q", status.InterviewStage)
	}
	if status.SentAt == nil || status.SentAt.String() != "2026-07-15" {
		t.Errorf("sent_at = %v, ожидалось 2026-07-15", status.SentAt)
	}
	if status.OfferedSalary == nil || *status.OfferedSalary != 350000 {
		t.Errorf("offered_salary = %v, ожидалось 350000", status.OfferedSalary)
	}
	if status.RealSalary == nil || *status.RealSalary != 320000.5 {
		t.Errorf("real_salary = %v, ожидалось 320000.5", status.RealSalary)
	}
	if status.CreatedAt.IsZero() || status.UpdatedAt.IsZero() {
		t.Error("created_at/updated_at не заполнены")
	}
}

func TestPutCandidateStatusUpsertsWithoutDuplicates(t *testing.T) {
	env := newTestEnv(t)

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/2"})
	path := "/api/vacancies/" + itoa(vacancy.ID) + "/candidate-status"

	var first candidateStatusResponse
	env.decode(env.request(http.MethodPut, path, fullStatusBody()), http.StatusOK, &first)

	body := fullStatusBody()
	body["interview_stage"] = "оффер обсуждается"
	body["offer_received"] = true

	var second candidateStatusResponse
	env.decode(env.request(http.MethodPut, path, body), http.StatusOK, &second)

	// Запись та же, меняется только содержимое.
	if second.ID != first.ID {
		t.Errorf("id = %d, ожидался прежний %d: связь 1:1", second.ID, first.ID)
	}
	if !second.CreatedAt.Equal(first.CreatedAt) {
		t.Errorf("created_at изменился: было %v, стало %v", first.CreatedAt, second.CreatedAt)
	}
	if second.InterviewStage != "оффер обсуждается" || !second.OfferReceived {
		t.Errorf("данные не обновились: %+v", second)
	}

	var count int64
	if err := env.db.Model(&model.CandidateStatus{}).Count(&count).Error; err != nil {
		t.Fatalf("подсчёт статусов: %v", err)
	}
	if count != 1 {
		t.Errorf("статусов в БД: %d, ожидался 1", count)
	}
}

func TestPutCandidateStatusReplacesOmittedFields(t *testing.T) {
	env := newTestEnv(t)

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/3"})
	path := "/api/vacancies/" + itoa(vacancy.ID) + "/candidate-status"

	env.decode(env.request(http.MethodPut, path, fullStatusBody()), http.StatusOK, nil)

	// PUT заменяет представление целиком: не присланные поля обнуляются.
	var replaced candidateStatusResponse
	env.decode(env.request(http.MethodPut, path, map[string]any{"interview_stage": "скрининг"}), http.StatusOK, &replaced)

	if replaced.InterviewStage != "скрининг" {
		t.Errorf("interview_stage = %q", replaced.InterviewStage)
	}
	if replaced.CoverLetter != "" {
		t.Errorf("cover_letter = %q, ожидалась пустая строка", replaced.CoverLetter)
	}
	if replaced.SentAt != nil {
		t.Errorf("sent_at = %v, ожидался null", replaced.SentAt)
	}
	if replaced.OfferedSalary != nil || replaced.RealSalary != nil {
		t.Errorf("зарплаты не обнулены: %v / %v", replaced.OfferedSalary, replaced.RealSalary)
	}
	if replaced.HRContact != "" || replaced.MarketSalaryData != "" || replaced.InterviewRecordURL != "" {
		t.Errorf("текстовые поля не обнулены: %+v", replaced)
	}
}

func TestPutCandidateStatusTrimsWhitespace(t *testing.T) {
	env := newTestEnv(t)

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/4"})

	var status candidateStatusResponse
	env.decode(
		env.request(http.MethodPut, "/api/vacancies/"+itoa(vacancy.ID)+"/candidate-status",
			map[string]any{"interview_stage": "  скрининг  ", "hr_contact": " @hr "}),
		http.StatusOK, &status,
	)

	if status.InterviewStage != "скрининг" || status.HRContact != "@hr" {
		t.Errorf("пробелы не обрезаны: %q / %q", status.InterviewStage, status.HRContact)
	}
}

func TestPutCandidateStatusValidation(t *testing.T) {
	env := newTestEnv(t)

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/5"})
	path := "/api/vacancies/" + itoa(vacancy.ID) + "/candidate-status"

	cases := []struct {
		name      string
		body      any
		wantField string
	}{
		{
			name:      "отрицательная предлагаемая ЗП",
			body:      map[string]any{"offered_salary": -100},
			wantField: "offered_salary",
		},
		{
			name:      "неправдоподобная реальная ЗП",
			body:      map[string]any{"real_salary": 1e12},
			wantField: "real_salary",
		},
		{
			name:      "ссылка на запись без схемы",
			body:      map[string]any{"interview_record_url": "example.com/record"},
			wantField: "interview_record_url",
		},
		{
			name:      "слишком длинный этап",
			body:      map[string]any{"interview_stage": longString(maxTextLen + 1)},
			wantField: "interview_stage",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := env.decodeError(env.request(http.MethodPut, path, tc.body), http.StatusBadRequest)

			if payload.Code != CodeValidationFailed {
				t.Errorf("code = %q, ожидалось %q", payload.Code, CodeValidationFailed)
			}
			if _, ok := payload.Fields[tc.wantField]; !ok {
				t.Errorf("в fields нет %q: %v", tc.wantField, payload.Fields)
			}
		})
	}
}

func TestPutCandidateStatusBadJSON(t *testing.T) {
	env := newTestEnv(t)

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/6"})
	path := "/api/vacancies/" + itoa(vacancy.ID) + "/candidate-status"

	cases := []struct {
		name string
		body string
	}{
		{name: "неверный формат даты", body: `{"sent_at":"15.07.2026"}`},
		{name: "неизвестное поле", body: `{"stage":"скрининг"}`},
		{name: "зарплата строкой", body: `{"offered_salary":"много"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := env.decodeError(env.request(http.MethodPut, path, tc.body), http.StatusBadRequest)
			if payload.Code != CodeInvalidJSON {
				t.Errorf("code = %q, ожидалось %q", payload.Code, CodeInvalidJSON)
			}
		})
	}
}

func TestPutCandidateStatusVacancyNotFound(t *testing.T) {
	env := newTestEnv(t)

	env.decodeError(
		env.request(http.MethodPut, "/api/vacancies/999/candidate-status", fullStatusBody()),
		http.StatusNotFound,
	)

	var count int64
	if err := env.db.Model(&model.CandidateStatus{}).Count(&count).Error; err != nil {
		t.Fatalf("подсчёт статусов: %v", err)
	}
	if count != 0 {
		t.Errorf("создано %d статусов для несуществующей вакансии, ожидалось 0", count)
	}
}

func TestGetCandidateStatus(t *testing.T) {
	env := newTestEnv(t)

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/7"})
	path := "/api/vacancies/" + itoa(vacancy.ID) + "/candidate-status"

	// Пока не заполнен — 404 с понятным сообщением.
	env.decodeError(env.request(http.MethodGet, path, nil), http.StatusNotFound)

	env.decode(env.request(http.MethodPut, path, fullStatusBody()), http.StatusOK, nil)

	var status candidateStatusResponse
	env.decode(env.request(http.MethodGet, path, nil), http.StatusOK, &status)
	if status.InterviewStage != "техническое интервью" {
		t.Errorf("interview_stage = %q", status.InterviewStage)
	}
}

func TestVacancyCardReturnsStatusInOneRequest(t *testing.T) {
	env := newTestEnv(t)

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/8"})
	env.decode(
		env.request(http.MethodPut, "/api/vacancies/"+itoa(vacancy.ID)+"/candidate-status", fullStatusBody()),
		http.StatusOK, nil,
	)

	// Карточка в UI должна получать вакансию и статус одним запросом.
	var card vacancyResponse
	env.decode(env.request(http.MethodGet, "/api/vacancies/"+itoa(vacancy.ID), nil), http.StatusOK, &card)

	if card.CandidateStatus == nil {
		t.Fatal("candidate_status = null, ожидался объект")
	}
	if card.CandidateStatus.HRContact != "@hr_example" {
		t.Errorf("hr_contact = %q", card.CandidateStatus.HRContact)
	}
	if card.CandidateStatus.SentAt == nil || card.CandidateStatus.SentAt.String() != "2026-07-15" {
		t.Errorf("sent_at = %v", card.CandidateStatus.SentAt)
	}
}

func TestPutCandidateStatusBumpsVacancyUpdatedAt(t *testing.T) {
	env := newTestEnv(t)

	first := env.createVacancy(map[string]any{"url": "https://example.com/jobs/one"})
	second := env.createVacancy(map[string]any{"url": "https://example.com/jobs/two"})

	touch(t, env, first.ID, time.Now().UTC().Add(-time.Hour))
	touch(t, env, second.ID, time.Now().UTC().Add(-2*time.Hour))

	// Заполнение статуса — правка карточки, вакансия должна подняться в списке.
	env.decode(
		env.request(http.MethodPut, "/api/vacancies/"+itoa(second.ID)+"/candidate-status",
			map[string]any{"interview_stage": "скрининг"}),
		http.StatusOK, nil,
	)

	items := env.listVacancies(t, "")
	if len(items) == 0 || items[0].ID != second.ID {
		t.Errorf("первой отдана вакансия %v, ожидалась %d", ids(items), second.ID)
	}
}

func TestDeleteVacancyRemovesStatusViaAPI(t *testing.T) {
	env := newTestEnv(t)

	vacancy := env.createVacancy(map[string]any{"url": "https://example.com/jobs/9"})
	env.decode(
		env.request(http.MethodPut, "/api/vacancies/"+itoa(vacancy.ID)+"/candidate-status", fullStatusBody()),
		http.StatusOK, nil,
	)

	rec := env.request(http.MethodDelete, "/api/vacancies/"+itoa(vacancy.ID), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("статус = %d, ожидался %d", rec.Code, http.StatusNoContent)
	}

	var count int64
	if err := env.db.Model(&model.CandidateStatus{}).Count(&count).Error; err != nil {
		t.Fatalf("подсчёт статусов: %v", err)
	}
	if count != 0 {
		t.Errorf("осталось статусов: %d, ожидалось 0", count)
	}
}

func longString(n int) string {
	buf := make([]byte, n)
	for i := range buf {
		buf[i] = 'a'
	}
	return string(buf)
}
