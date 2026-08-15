package api

import (
	"net/http"
	"testing"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

func TestCreateVacancyWithNewFields(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{
		"url":             "https://example.com/jobs/full",
		"title":           "  Go-разработчик  ",
		"company":         "Example",
		"grade":           model.GradeSenior,
		"salary_from":     300000,
		"salary_to":       450000,
		"salary_currency": "rub",
		"salary_gross":    true,
		"work_format":     model.WorkFormatRemote,
	})

	if created.Title != "Go-разработчик" {
		t.Errorf("title = %q, ожидалось %q (пробелы обрезаются)", created.Title, "Go-разработчик")
	}
	if created.SalaryFrom == nil || *created.SalaryFrom != 300000 {
		t.Errorf("salary_from = %v, ожидалось 300000", created.SalaryFrom)
	}
	if created.SalaryTo == nil || *created.SalaryTo != 450000 {
		t.Errorf("salary_to = %v, ожидалось 450000", created.SalaryTo)
	}
	// Код валюты приводится к верхнему регистру: «rub» и «RUB» — одно и то же.
	if created.SalaryCurrency != "RUB" {
		t.Errorf("salary_currency = %q, ожидалось RUB", created.SalaryCurrency)
	}
	if created.SalaryGross == nil || !*created.SalaryGross {
		t.Errorf("salary_gross = %v, ожидалось true", created.SalaryGross)
	}
	if created.WorkFormat != model.WorkFormatRemote {
		t.Errorf("work_format = %q, ожидалось %q", created.WorkFormat, model.WorkFormatRemote)
	}
}

func TestCreateVacancyNewFieldsEmpty(t *testing.T) {
	env := newTestEnv(t)

	// Базовый сценарий Этапа 1 не должен пострадать: новые поля необязательны.
	created := env.createVacancy(map[string]any{"url": "https://example.com/jobs/bare"})

	if created.Title != "" || created.WorkFormat != "" || created.SalaryCurrency != "" {
		t.Errorf("новые текстовые поля не пусты: %+v", created)
	}
	if created.SalaryFrom != nil || created.SalaryTo != nil || created.SalaryGross != nil {
		t.Errorf("зарплатные поля не пусты: %v / %v / %v",
			created.SalaryFrom, created.SalaryTo, created.SalaryGross)
	}
}

func TestCreateVacancyOneSidedSalary(t *testing.T) {
	env := newTestEnv(t)

	t.Run("только нижняя граница", func(t *testing.T) {
		created := env.createVacancy(map[string]any{
			"url":         "https://example.com/jobs/from",
			"salary_from": 250000,
		})
		if created.SalaryTo != nil {
			t.Errorf("salary_to = %v, ожидался null", *created.SalaryTo)
		}
		// Валюта подставляется сама: вилка без валюты бессмысленна.
		if created.SalaryCurrency != model.DefaultSalaryCurrency {
			t.Errorf("salary_currency = %q, ожидалось %q", created.SalaryCurrency, model.DefaultSalaryCurrency)
		}
	})

	t.Run("только верхняя граница", func(t *testing.T) {
		created := env.createVacancy(map[string]any{
			"url":       "https://example.com/jobs/to",
			"salary_to": 500000,
		})
		if created.SalaryFrom != nil {
			t.Errorf("salary_from = %v, ожидался null", *created.SalaryFrom)
		}
		if created.SalaryCurrency != model.DefaultSalaryCurrency {
			t.Errorf("salary_currency = %q, ожидалось %q", created.SalaryCurrency, model.DefaultSalaryCurrency)
		}
	})
}

func TestCreateVacancyCurrencyWithoutSalaryIsDropped(t *testing.T) {
	env := newTestEnv(t)

	// Валюта и «до вычета налогов» без вилки ничего не означают.
	created := env.createVacancy(map[string]any{
		"url":             "https://example.com/jobs/no-salary",
		"salary_currency": "USD",
		"salary_gross":    true,
	})

	if created.SalaryCurrency != "" {
		t.Errorf("salary_currency = %q, ожидалась пустая строка без вилки", created.SalaryCurrency)
	}
	if created.SalaryGross != nil {
		t.Errorf("salary_gross = %v, ожидался null без вилки", *created.SalaryGross)
	}
}

func TestCreateVacancyNewFieldsValidation(t *testing.T) {
	env := newTestEnv(t)

	cases := []struct {
		name      string
		body      any
		wantField string
	}{
		{
			name:      "верхняя граница меньше нижней",
			body:      map[string]any{"url": "https://example.com/j", "salary_from": 400000, "salary_to": 100000},
			wantField: "salary_to",
		},
		{
			name:      "отрицательная нижняя граница",
			body:      map[string]any{"url": "https://example.com/j", "salary_from": -1},
			wantField: "salary_from",
		},
		{
			name:      "неправдоподобная верхняя граница",
			body:      map[string]any{"url": "https://example.com/j", "salary_to": 1e12},
			wantField: "salary_to",
		},
		{
			name:      "валюта не из трёх букв",
			body:      map[string]any{"url": "https://example.com/j", "salary_from": 1, "salary_currency": "рубли"},
			wantField: "salary_currency",
		},
		{
			name:      "валюта из двух букв",
			body:      map[string]any{"url": "https://example.com/j", "salary_from": 1, "salary_currency": "RU"},
			wantField: "salary_currency",
		},
		{
			name:      "неизвестный формат работы",
			body:      map[string]any{"url": "https://example.com/j", "work_format": "из дома"},
			wantField: "work_format",
		},
		{
			name:      "слишком длинная должность",
			body:      map[string]any{"url": "https://example.com/j", "title": longString(maxTextLen + 1)},
			wantField: "title",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := env.decodeError(env.request(http.MethodPost, "/api/vacancies", tc.body), http.StatusBadRequest)

			if payload.Code != CodeValidationFailed {
				t.Errorf("code = %q, ожидалось %q", payload.Code, CodeValidationFailed)
			}
			if _, ok := payload.Fields[tc.wantField]; !ok {
				t.Errorf("в fields нет %q: %v", tc.wantField, payload.Fields)
			}
		})
	}
}

func TestCreateVacancyEqualSalaryBoundsAllowed(t *testing.T) {
	env := newTestEnv(t)

	// Фиксированная зарплата — это тоже вилка, просто вырожденная.
	created := env.createVacancy(map[string]any{
		"url":         "https://example.com/jobs/fixed",
		"salary_from": 350000,
		"salary_to":   350000,
	})

	if created.SalaryFrom == nil || created.SalaryTo == nil || *created.SalaryFrom != *created.SalaryTo {
		t.Errorf("границы не совпали: %v / %v", created.SalaryFrom, created.SalaryTo)
	}
}

func TestUpdateVacancyNewFields(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{
		"url":             "https://example.com/jobs/patch",
		"title":           "Go-разработчик",
		"salary_from":     300000,
		"salary_to":       400000,
		"salary_currency": "RUB",
		"work_format":     model.WorkFormatOnsite,
	})
	path := "/api/vacancies/" + itoa(created.ID)

	var updated vacancyResponse
	env.decode(
		env.request(http.MethodPatch, path, map[string]any{
			"title":       "Senior Go Engineer",
			"work_format": model.WorkFormatHybrid,
		}),
		http.StatusOK, &updated,
	)

	if updated.Title != "Senior Go Engineer" || updated.WorkFormat != model.WorkFormatHybrid {
		t.Errorf("поля не обновились: %+v", updated)
	}
	// Не присланные поля остаются как были.
	if updated.SalaryFrom == nil || *updated.SalaryFrom != 300000 {
		t.Errorf("salary_from = %v, ожидалось прежнее 300000", updated.SalaryFrom)
	}
}

func TestUpdateVacancyClearNewFields(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{
		"url":             "https://example.com/jobs/clear",
		"title":           "Go-разработчик",
		"salary_from":     300000,
		"salary_to":       400000,
		"salary_currency": "USD",
		"salary_gross":    true,
		"work_format":     model.WorkFormatRemote,
	})

	var cleared vacancyResponse
	env.decode(
		env.request(http.MethodPatch, "/api/vacancies/"+itoa(created.ID),
			`{"title":null,"salary_from":null,"salary_to":null,"work_format":null}`),
		http.StatusOK, &cleared,
	)

	if cleared.Title != "" || cleared.WorkFormat != "" {
		t.Errorf("текстовые поля не очищены: %+v", cleared)
	}
	if cleared.SalaryFrom != nil || cleared.SalaryTo != nil {
		t.Errorf("вилка не очищена: %v / %v", cleared.SalaryFrom, cleared.SalaryTo)
	}
	// Вместе с вилкой уходят валюта и признак «до вычета налогов».
	if cleared.SalaryCurrency != "" {
		t.Errorf("salary_currency = %q, ожидалась пустая строка", cleared.SalaryCurrency)
	}
	if cleared.SalaryGross != nil {
		t.Errorf("salary_gross = %v, ожидался null", *cleared.SalaryGross)
	}
}

func TestUpdateVacancySalaryConsistencyAcrossRequests(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{
		"url":         "https://example.com/jobs/consistency",
		"salary_from": 400000,
	})
	path := "/api/vacancies/" + itoa(created.ID)

	// Нижняя граница уже в базе, верхняя приходит в PATCH — проверка должна
	// сработать на итоговом значении, а не на теле запроса.
	payload := env.decodeError(
		env.request(http.MethodPatch, path, map[string]any{"salary_to": 100000}),
		http.StatusBadRequest,
	)
	if _, ok := payload.Fields["salary_to"]; !ok {
		t.Errorf("в fields нет salary_to: %v", payload.Fields)
	}

	// Убедимся, что отклонённый запрос ничего не записал.
	var unchanged vacancyResponse
	env.decode(env.request(http.MethodGet, path, nil), http.StatusOK, &unchanged)
	if unchanged.SalaryTo != nil {
		t.Errorf("salary_to = %v, ожидался null: запрос был отклонён", *unchanged.SalaryTo)
	}
}

func TestUpdateVacancyClearingLowerBoundKeepsUpper(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{
		"url":         "https://example.com/jobs/half",
		"salary_from": 300000,
		"salary_to":   400000,
	})

	// «До 400k» — осмысленное объявление, очистка одной границы допустима.
	var updated vacancyResponse
	env.decode(
		env.request(http.MethodPatch, "/api/vacancies/"+itoa(created.ID), `{"salary_from":null}`),
		http.StatusOK, &updated,
	)

	if updated.SalaryFrom != nil {
		t.Errorf("salary_from = %v, ожидался null", *updated.SalaryFrom)
	}
	if updated.SalaryTo == nil || *updated.SalaryTo != 400000 {
		t.Errorf("salary_to = %v, ожидалось 400000", updated.SalaryTo)
	}
	if updated.SalaryCurrency != model.DefaultSalaryCurrency {
		t.Errorf("salary_currency = %q, ожидалось %q: вилка ещё есть",
			updated.SalaryCurrency, model.DefaultSalaryCurrency)
	}
}

func TestUpdateVacancyNewFieldsValidation(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{"url": "https://example.com/jobs/validate"})
	path := "/api/vacancies/" + itoa(created.ID)

	cases := []struct {
		name      string
		body      any
		wantField string
	}{
		{name: "неизвестный формат", body: map[string]any{"work_format": "гибридный"}, wantField: "work_format"},
		{name: "битая валюта", body: map[string]any{"salary_currency": "рубль"}, wantField: "salary_currency"},
		{name: "отрицательная граница", body: map[string]any{"salary_to": -5}, wantField: "salary_to"},
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

func TestVacancyNewFieldsPersistAndReachList(t *testing.T) {
	env := newTestEnv(t)

	env.createVacancy(map[string]any{
		"url":         "https://example.com/jobs/list",
		"title":       "Go-разработчик",
		"salary_from": 300000,
		"work_format": model.WorkFormatRemote,
	})

	// Таблица в UI показывает вилку и формат, значит список обязан их отдавать.
	items := env.listVacancies(t, "")
	if len(items) != 1 {
		t.Fatalf("вакансий в списке: %d", len(items))
	}
	if items[0].Title != "Go-разработчик" {
		t.Errorf("title = %q", items[0].Title)
	}
	if items[0].SalaryFrom == nil || *items[0].SalaryFrom != 300000 {
		t.Errorf("salary_from = %v", items[0].SalaryFrom)
	}
	if items[0].WorkFormat != model.WorkFormatRemote {
		t.Errorf("work_format = %q", items[0].WorkFormat)
	}

	// И в БД тоже: ответ мог бы собраться из памяти.
	var stored model.Vacancy
	if err := env.db.First(&stored, items[0].ID).Error; err != nil {
		t.Fatalf("чтение вакансии: %v", err)
	}
	if stored.Title != "Go-разработчик" || stored.WorkFormat != model.WorkFormatRemote {
		t.Errorf("в БД поля не совпали: %+v", stored)
	}
	if stored.SalaryCurrency != model.DefaultSalaryCurrency {
		t.Errorf("salary_currency в БД = %q, ожидалось %q", stored.SalaryCurrency, model.DefaultSalaryCurrency)
	}
}

func TestSalaryGrossRoundTrip(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{
		"url":          "https://example.com/jobs/gross",
		"salary_from":  300000,
		"salary_gross": false,
	})
	if created.SalaryGross == nil || *created.SalaryGross {
		t.Fatalf("salary_gross = %v, ожидалось false (на руки)", created.SalaryGross)
	}

	var updated vacancyResponse
	env.decode(
		env.request(http.MethodPatch, "/api/vacancies/"+itoa(created.ID), `{"salary_gross":null}`),
		http.StatusOK, &updated,
	)
	if updated.SalaryGross != nil {
		t.Errorf("salary_gross = %v, ожидался null (не указано)", *updated.SalaryGross)
	}
}
