package api

import (
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

func TestCreateVacancy(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{
		"url":         "https://example.com/jobs/1",
		"company":     "  Example  ",
		"grade":       model.GradeSenior,
		"tech_tags":   []string{"go", " docker ", ""},
		"opened_date": "2026-08-01",
	})

	if created.ID == 0 {
		t.Fatal("id не присвоен")
	}
	if created.Company != "Example" {
		t.Errorf("company = %q, ожидалось %q (пробелы должны обрезаться)", created.Company, "Example")
	}
	if got, want := []string(created.TechTags), []string{"go", "docker"}; len(got) != len(want) {
		t.Errorf("tech_tags = %v, ожидалось %v (пустые теги отбрасываются)", got, want)
	}
	if created.OpenedDate == nil || created.OpenedDate.String() != "2026-08-01" {
		t.Errorf("opened_date = %v, ожидалось 2026-08-01", created.OpenedDate)
	}
	// Свежая вакансия считается активной, хотя проверок ещё не было.
	if !created.IsActive {
		t.Error("is_active = false, ожидалось true для новой вакансии")
	}
	if created.AutoIsActive != nil || created.ManualIsActive != nil {
		t.Errorf("auto/manual = %v/%v, ожидались nil", created.AutoIsActive, created.ManualIsActive)
	}
	if created.ActivityConflict {
		t.Error("activity_conflict = true, ожидалось false")
	}
	if created.CandidateStatus != nil {
		t.Error("candidate_status заполнен, ожидался null")
	}
}

func TestCreateVacancyOnlyURL(t *testing.T) {
	env := newTestEnv(t)

	// Базовый сценарий Этапа 1: вставил ссылку, остальное потом.
	created := env.createVacancy(map[string]any{"url": "https://example.com/jobs/2"})

	if created.Grade != "" || created.Company != "" {
		t.Errorf("необязательные поля не пусты: %+v", created)
	}
	if created.TechTags == nil {
		t.Error("tech_tags = null, ожидался пустой массив")
	}
	if created.OpenedDate != nil {
		t.Errorf("opened_date = %v, ожидался null", created.OpenedDate)
	}
}

func TestCreateVacancyValidation(t *testing.T) {
	env := newTestEnv(t)

	cases := []struct {
		name      string
		body      any
		wantField string
	}{
		{name: "нет url", body: map[string]any{"company": "Example"}, wantField: "url"},
		{name: "пустой url", body: map[string]any{"url": "   "}, wantField: "url"},
		{name: "url без схемы", body: map[string]any{"url": "example.com/jobs/1"}, wantField: "url"},
		{name: "неподдерживаемая схема", body: map[string]any{"url": "ftp://example.com/jobs"}, wantField: "url"},
		{name: "url без домена", body: map[string]any{"url": "https:///jobs"}, wantField: "url"},
		{name: "неизвестный грейд", body: map[string]any{"url": "https://example.com/j", "grade": "god"}, wantField: "grade"},
		{name: "грейд не в том регистре", body: map[string]any{"url": "https://example.com/j", "grade": "Senior"}, wantField: "grade"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := env.request(http.MethodPost, "/api/vacancies", tc.body)
			payload := env.decodeError(rec, http.StatusBadRequest)

			if payload.Code != CodeValidationFailed {
				t.Errorf("code = %q, ожидалось %q", payload.Code, CodeValidationFailed)
			}
			if _, ok := payload.Fields[tc.wantField]; !ok {
				t.Errorf("в fields нет %q: %v", tc.wantField, payload.Fields)
			}
		})
	}
}

func TestCreateVacancyBadJSON(t *testing.T) {
	env := newTestEnv(t)

	cases := []struct {
		name string
		body string
	}{
		{name: "битый json", body: `{"url": `},
		{name: "неизвестное поле", body: `{"url":"https://example.com/j","colour":"red"}`},
		{name: "неверный тип поля", body: `{"url": 42}`},
		{name: "неверный формат даты", body: `{"url":"https://example.com/j","opened_date":"01.08.2026"}`},
		{name: "два объекта в теле", body: `{"url":"https://example.com/j"}{"url":"https://example.com/k"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := env.request(http.MethodPost, "/api/vacancies", tc.body)
			payload := env.decodeError(rec, http.StatusBadRequest)

			if payload.Code != CodeInvalidJSON {
				t.Errorf("code = %q, ожидалось %q", payload.Code, CodeInvalidJSON)
			}
		})
	}
}

func TestGetVacancy(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{"url": "https://example.com/jobs/3", "company": "Example"})

	var got vacancyResponse
	env.decode(env.request(http.MethodGet, "/api/vacancies/"+itoa(created.ID), nil), http.StatusOK, &got)

	if got.ID != created.ID || got.Company != "Example" {
		t.Errorf("получено %+v, ожидалась вакансия %d", got, created.ID)
	}
}

func TestGetVacancyNotFound(t *testing.T) {
	env := newTestEnv(t)

	for _, path := range []string{"/api/vacancies/999", "/api/vacancies/0", "/api/vacancies/abc", "/api/vacancies/-1"} {
		t.Run(path, func(t *testing.T) {
			payload := env.decodeError(env.request(http.MethodGet, path, nil), http.StatusNotFound)
			if payload.Code != CodeNotFound {
				t.Errorf("code = %q, ожидалось %q", payload.Code, CodeNotFound)
			}
		})
	}
}

func TestUpdateVacancyPartial(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{
		"url":         "https://example.com/jobs/4",
		"company":     "Example",
		"grade":       model.GradeMiddle,
		"tech_tags":   []string{"go"},
		"opened_date": "2026-08-01",
	})

	var updated vacancyResponse
	env.decode(
		env.request(http.MethodPatch, "/api/vacancies/"+itoa(created.ID), map[string]any{"grade": model.GradeSenior}),
		http.StatusOK, &updated,
	)

	if updated.Grade != model.GradeSenior {
		t.Errorf("grade = %q, ожидалось %q", updated.Grade, model.GradeSenior)
	}
	// Не присланные поля должны остаться как были.
	if updated.Company != "Example" {
		t.Errorf("company = %q, ожидалось Example", updated.Company)
	}
	if len(updated.TechTags) != 1 || updated.TechTags[0] != "go" {
		t.Errorf("tech_tags = %v, ожидалось [go]", updated.TechTags)
	}
	if updated.OpenedDate == nil || updated.OpenedDate.String() != "2026-08-01" {
		t.Errorf("opened_date = %v, ожидалось 2026-08-01", updated.OpenedDate)
	}
}

func TestUpdateVacancyClearFields(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{
		"url":         "https://example.com/jobs/5",
		"company":     "Example",
		"grade":       model.GradeMiddle,
		"tech_tags":   []string{"go", "docker"},
		"opened_date": "2026-08-01",
	})

	// null означает «очистить», в отличие от отсутствия поля.
	body := `{"company":null,"grade":null,"tech_tags":null,"opened_date":null}`

	var updated vacancyResponse
	env.decode(env.request(http.MethodPatch, "/api/vacancies/"+itoa(created.ID), body), http.StatusOK, &updated)

	if updated.Company != "" || updated.Grade != "" {
		t.Errorf("поля не очищены: %+v", updated)
	}
	if len(updated.TechTags) != 0 {
		t.Errorf("tech_tags = %v, ожидался пустой массив", updated.TechTags)
	}
	if updated.OpenedDate != nil {
		t.Errorf("opened_date = %v, ожидался null", updated.OpenedDate)
	}
	// Ссылка при этом должна остаться: вакансия без url бессмысленна.
	if updated.URL != created.URL {
		t.Errorf("url = %q, ожидалось %q", updated.URL, created.URL)
	}
}

func TestUpdateVacancyValidation(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{"url": "https://example.com/jobs/6"})
	path := "/api/vacancies/" + itoa(created.ID)

	cases := []struct {
		name      string
		body      any
		wantField string
	}{
		{name: "url нельзя очистить", body: `{"url":null}`, wantField: "url"},
		{name: "битый url", body: map[string]any{"url": "не ссылка"}, wantField: "url"},
		{name: "неизвестный грейд", body: map[string]any{"grade": "god"}, wantField: "grade"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := env.decodeError(env.request(http.MethodPatch, path, tc.body), http.StatusBadRequest)
			if _, ok := payload.Fields[tc.wantField]; !ok {
				t.Errorf("в fields нет %q: %v", tc.wantField, payload.Fields)
			}
		})
	}
}

func TestUpdateVacancyNotFound(t *testing.T) {
	env := newTestEnv(t)

	rec := env.request(http.MethodPatch, "/api/vacancies/999", map[string]any{"company": "Example"})
	env.decodeError(rec, http.StatusNotFound)
}

func TestDeleteVacancy(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{"url": "https://example.com/jobs/7"})
	path := "/api/vacancies/" + itoa(created.ID)

	rec := env.request(http.MethodDelete, path, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("статус = %d, ожидался %d, тело: %s", rec.Code, http.StatusNoContent, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("тело ответа не пустое: %s", rec.Body.String())
	}

	env.decodeError(env.request(http.MethodGet, path, nil), http.StatusNotFound)
	// Повторное удаление — уже 404.
	env.decodeError(env.request(http.MethodDelete, path, nil), http.StatusNotFound)
}

func TestDeleteVacancyRemovesCandidateStatus(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{"url": "https://example.com/jobs/8"})

	// CRUD статуса появится в Task 6, поэтому связанную запись создаём напрямую.
	status := model.CandidateStatus{VacancyID: created.ID, InterviewStage: "отклик отправлен"}
	if err := env.db.Create(&status).Error; err != nil {
		t.Fatalf("создание статуса: %v", err)
	}

	rec := env.request(http.MethodDelete, "/api/vacancies/"+itoa(created.ID), nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("статус = %d, ожидался %d", rec.Code, http.StatusNoContent)
	}

	var count int64
	if err := env.db.Model(&model.CandidateStatus{}).Count(&count).Error; err != nil {
		t.Fatalf("подсчёт статусов: %v", err)
	}
	if count != 0 {
		t.Errorf("осталось статусов: %d, ожидалось 0 (каскадное удаление)", count)
	}
}

func TestListVacanciesSortedByUpdatedAtDesc(t *testing.T) {
	env := newTestEnv(t)

	first := env.createVacancy(map[string]any{"url": "https://example.com/jobs/first"})
	second := env.createVacancy(map[string]any{"url": "https://example.com/jobs/second"})

	// updated_at в SQLite хранится с высокой точностью, но на быстрой машине
	// две записи могут получить одну метку — сдвигаем время явно.
	touch(t, env, first.ID, time.Now().UTC().Add(-time.Hour))
	touch(t, env, second.ID, time.Now().UTC().Add(-2*time.Hour))

	items := env.listVacancies(t, "")
	if len(items) != 2 {
		t.Fatalf("вакансий в списке: %d, ожидалось 2", len(items))
	}
	// Свежеизменённая — сверху (сортировка по умолчанию из п. 6 ТЗ).
	if items[0].ID != first.ID {
		t.Errorf("первой отдана вакансия %d, ожидалась %d", items[0].ID, first.ID)
	}

	asc := env.listVacancies(t, "?order=asc")
	if asc[0].ID != second.ID {
		t.Errorf("при order=asc первой отдана %d, ожидалась %d", asc[0].ID, second.ID)
	}
}

func TestListVacanciesHidesInactive(t *testing.T) {
	env := newTestEnv(t)

	active := env.createVacancy(map[string]any{"url": "https://example.com/jobs/active"})
	autoClosed := env.createVacancy(map[string]any{"url": "https://example.com/jobs/auto-closed"})
	manualClosed := env.createVacancy(map[string]any{"url": "https://example.com/jobs/manual-closed"})
	// Сайт отдал 404, но пользователь считает вакансию живой — она должна остаться в списке.
	overriddenActive := env.createVacancy(map[string]any{"url": "https://example.com/jobs/overridden"})

	setActivity(t, env, autoClosed.ID, boolPtr(false), nil)
	setActivity(t, env, manualClosed.ID, nil, boolPtr(false))
	setActivity(t, env, overriddenActive.ID, boolPtr(false), boolPtr(true))

	visible := env.listVacancies(t, "")
	visibleIDs := idSet(visible)

	if !visibleIDs[active.ID] {
		t.Error("активная вакансия скрыта")
	}
	if !visibleIDs[overriddenActive.ID] {
		t.Error("вакансия с ручным override «активна» скрыта, хотя проверка дала 404")
	}
	if visibleIDs[autoClosed.ID] {
		t.Error("вакансия, снятая по авто-проверке, показана без include_inactive")
	}
	if visibleIDs[manualClosed.ID] {
		t.Error("вакансия, закрытая вручную, показана без include_inactive")
	}

	all := env.listVacancies(t, "?include_inactive=true")
	if len(all) != 4 {
		t.Errorf("с include_inactive=true отдано %d вакансий, ожидалось 4", len(all))
	}

	// Флаг расхождения должен доехать до клиента.
	for _, item := range all {
		if item.ID == overriddenActive.ID && !item.ActivityConflict {
			t.Error("activity_conflict = false, хотя решение пользователя расходится с проверкой")
		}
	}
}

func TestListVacanciesEmpty(t *testing.T) {
	env := newTestEnv(t)

	var resp listVacanciesResponse
	env.decode(env.request(http.MethodGet, "/api/vacancies", nil), http.StatusOK, &resp)

	if resp.Items == nil {
		t.Error("items = null, ожидался пустой массив")
	}
	if len(resp.Items) != 0 {
		t.Errorf("items = %v, ожидался пустой список", resp.Items)
	}
}

func TestListVacanciesQueryValidation(t *testing.T) {
	env := newTestEnv(t)

	cases := []struct {
		query     string
		wantField string
	}{
		{query: "?sort=drop_table", wantField: "sort"},
		{query: "?order=random", wantField: "order"},
		{query: "?include_inactive=ага", wantField: "include_inactive"},
	}

	for _, tc := range cases {
		t.Run(tc.query, func(t *testing.T) {
			payload := env.decodeError(env.request(http.MethodGet, "/api/vacancies"+tc.query, nil), http.StatusBadRequest)
			if _, ok := payload.Fields[tc.wantField]; !ok {
				t.Errorf("в fields нет %q: %v", tc.wantField, payload.Fields)
			}
		})
	}
}

func TestListVacanciesSortByCompany(t *testing.T) {
	env := newTestEnv(t)

	env.createVacancy(map[string]any{"url": "https://example.com/jobs/b", "company": "Bravo"})
	env.createVacancy(map[string]any{"url": "https://example.com/jobs/a", "company": "Alpha"})

	items := env.listVacancies(t, "?sort=company&order=asc")
	if len(items) != 2 || items[0].Company != "Alpha" {
		t.Errorf("порядок по company asc неверный: %v", companies(items))
	}
}

func TestListVacanciesIncludesCandidateStatus(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{"url": "https://example.com/jobs/9"})

	sent := model.NewDate(2026, time.July, 15)
	status := model.CandidateStatus{
		VacancyID:      created.ID,
		SentAt:         &sent,
		InterviewStage: "техническое интервью",
	}
	if err := env.db.Create(&status).Error; err != nil {
		t.Fatalf("создание статуса: %v", err)
	}

	// Таблица в UI показывает дату отклика и этап, значит список обязан их отдавать.
	items := env.listVacancies(t, "")
	if len(items) != 1 {
		t.Fatalf("вакансий в списке: %d", len(items))
	}
	got := items[0].CandidateStatus
	if got == nil {
		t.Fatal("candidate_status = null, ожидался объект")
	}
	if got.InterviewStage != "техническое интервью" {
		t.Errorf("interview_stage = %q", got.InterviewStage)
	}
	if got.SentAt == nil || got.SentAt.String() != "2026-07-15" {
		t.Errorf("sent_at = %v, ожидалось 2026-07-15", got.SentAt)
	}
}

func TestUnknownAPIRouteReturnsJSON(t *testing.T) {
	env := newTestEnv(t)

	payload := env.decodeError(env.request(http.MethodGet, "/api/unknown", nil), http.StatusNotFound)
	if payload.Code != CodeNotFound {
		t.Errorf("code = %q, ожидалось %q", payload.Code, CodeNotFound)
	}
}

func TestMethodNotAllowed(t *testing.T) {
	env := newTestEnv(t)

	rec := env.request(http.MethodPut, "/api/vacancies", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("статус = %d, ожидался %d", rec.Code, http.StatusMethodNotAllowed)
	}
}

// --- вспомогательные функции ---

func (e *testEnv) listVacancies(t *testing.T, query string) []vacancyResponse {
	t.Helper()

	var resp listVacanciesResponse
	e.decode(e.request(http.MethodGet, "/api/vacancies"+query, nil), http.StatusOK, &resp)
	return resp.Items
}

// setActivity выставляет поля активности напрямую: эндпоинт PUT /activity — это Task 5.
func setActivity(t *testing.T, env *testEnv, id uint, auto, manual *bool) {
	t.Helper()

	err := env.db.Model(&model.Vacancy{}).Where("id = ?", id).
		UpdateColumns(map[string]any{"auto_is_active": auto, "manual_is_active": manual}).Error
	if err != nil {
		t.Fatalf("обновление активности: %v", err)
	}
}

// touch задаёт updated_at напрямую, чтобы проверить сортировку без ожиданий в тесте.
func touch(t *testing.T, env *testEnv, id uint, at time.Time) {
	t.Helper()

	err := env.db.Model(&model.Vacancy{}).Where("id = ?", id).
		UpdateColumn("updated_at", at).Error
	if err != nil {
		t.Fatalf("обновление updated_at: %v", err)
	}
}

func idSet(items []vacancyResponse) map[uint]bool {
	set := make(map[uint]bool, len(items))
	for _, item := range items {
		set[item.ID] = true
	}
	return set
}

func companies(items []vacancyResponse) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Company)
	}
	return names
}

func boolPtr(v bool) *bool { return &v }

func itoa(id uint) string {
	return strconv.FormatUint(uint64(id), 10)
}
