package store

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

// makeRun собирает запись журнала с настраиваемыми полями.
func makeRun(status string, vacancyID *uint, input, output int, cost *float64) model.LLMRun {
	return model.LLMRun{
		Purpose:       model.PurposeExtractVacancy,
		VacancyID:     vacancyID,
		Provider:      "gemini",
		Model:         "gemini-2.5-flash",
		PromptVersion: "extract-v1",
		SourceURL:     "https://example.com/vacancy",
		SourceChars:   2900,
		Status:        status,
		InputTokens:   &input,
		OutputTokens:  &output,
		CostEstimate:  cost,
		Attempts:      1,
		DurationMs:    1200,
		ResponseJSON:  `{"title":"Go-разработчик"}`,
	}
}

func createVacancyForRuns(t *testing.T, db *gorm.DB, title string) uint {
	t.Helper()

	vacancy := model.Vacancy{URL: "https://example.com/" + title, Title: title}
	if err := CreateVacancy(db, &vacancy); err != nil {
		t.Fatalf("создание вакансии: %v", err)
	}
	return vacancy.ID
}

func TestCreateAndGetLLMRun(t *testing.T) {
	db := newMemoryDB(t)

	vacancyID := createVacancyForRuns(t, db, "Go-разработчик")
	cost := 0.0032
	run := makeRun(model.RunStatusOK, &vacancyID, 1071, 106, &cost)

	if err := CreateLLMRun(db, &run); err != nil {
		t.Fatalf("CreateLLMRun: %v", err)
	}
	if run.ID == 0 {
		t.Fatal("id не присвоен")
	}
	if run.CreatedAt.IsZero() {
		t.Error("created_at не заполнен")
	}

	got, err := GetLLMRun(db, run.ID)
	if err != nil {
		t.Fatalf("GetLLMRun: %v", err)
	}

	if got.Provider != "gemini" || got.Model != "gemini-2.5-flash" {
		t.Errorf("провайдер и модель: %+v", got)
	}
	// Сырой ответ доступен в карточке одного запуска.
	if got.ResponseJSON == "" {
		t.Error("сырой ответ модели не сохранён")
	}
	// Вакансия подгружается для подписи на экране.
	if got.Vacancy == nil || got.Vacancy.Title != "Go-разработчик" {
		t.Errorf("вакансия не подгружена: %+v", got.Vacancy)
	}
}

func TestGetLLMRunNotFound(t *testing.T) {
	db := newMemoryDB(t)

	if _, err := GetLLMRun(db, 999); err != ErrNotFound {
		t.Fatalf("err = %v, ожидалась ErrNotFound", err)
	}
}

func TestListLLMRunsNewestFirst(t *testing.T) {
	db := newMemoryDB(t)

	for i := 0; i < 3; i++ {
		run := makeRun(model.RunStatusOK, nil, 100, 10, nil)
		if err := CreateLLMRun(db, &run); err != nil {
			t.Fatalf("создание записи: %v", err)
		}
		// Метки времени в SQLite точные, но на быстрой машине могут совпасть:
		// порядок должен держаться и по id.
		if err := db.Model(&model.LLMRun{}).Where("id = ?", run.ID).
			UpdateColumn("created_at", time.Now().UTC().Add(-time.Duration(3-i)*time.Hour)).Error; err != nil {
			t.Fatalf("сдвиг времени: %v", err)
		}
	}

	runs, count, err := ListLLMRuns(db, LLMRunFilter{})
	if err != nil {
		t.Fatalf("ListLLMRuns: %v", err)
	}
	if count != 3 || len(runs) != 3 {
		t.Fatalf("count = %d, отдано %d", count, len(runs))
	}
	for i := 1; i < len(runs); i++ {
		if runs[i-1].CreatedAt.Before(runs[i].CreatedAt) {
			t.Errorf("порядок нарушен: %v раньше %v", runs[i-1].CreatedAt, runs[i].CreatedAt)
		}
	}
}

func TestListLLMRunsOmitsRawResponse(t *testing.T) {
	db := newMemoryDB(t)

	run := makeRun(model.RunStatusOK, nil, 100, 10, nil)
	if err := CreateLLMRun(db, &run); err != nil {
		t.Fatalf("создание записи: %v", err)
	}

	runs, _, err := ListLLMRuns(db, LLMRunFilter{})
	if err != nil {
		t.Fatalf("ListLLMRuns: %v", err)
	}

	// Сырой ответ бывает большим, а на экране журнала не нужен.
	if runs[0].ResponseJSON != "" {
		t.Errorf("в списке пришёл сырой ответ: %q", runs[0].ResponseJSON)
	}
	// Остальное на месте.
	if runs[0].Status != model.RunStatusOK {
		t.Errorf("статус = %q", runs[0].Status)
	}
}

func TestListLLMRunsFilterByVacancy(t *testing.T) {
	db := newMemoryDB(t)

	first := createVacancyForRuns(t, db, "первая")
	second := createVacancyForRuns(t, db, "вторая")

	for _, id := range []uint{first, first, second} {
		vacancyID := id
		run := makeRun(model.RunStatusOK, &vacancyID, 100, 10, nil)
		if err := CreateLLMRun(db, &run); err != nil {
			t.Fatalf("создание записи: %v", err)
		}
	}
	// Запуск без вакансии тоже есть: так выглядит история после её удаления.
	orphan := makeRun(model.RunStatusFetchError, nil, 0, 0, nil)
	if err := CreateLLMRun(db, &orphan); err != nil {
		t.Fatalf("создание записи: %v", err)
	}

	runs, count, err := ListLLMRuns(db, LLMRunFilter{VacancyID: &first})
	if err != nil {
		t.Fatalf("ListLLMRuns: %v", err)
	}
	if count != 2 || len(runs) != 2 {
		t.Fatalf("count = %d, отдано %d, ожидалось 2", count, len(runs))
	}
	for _, run := range runs {
		if run.VacancyID == nil || *run.VacancyID != first {
			t.Errorf("в выборку попал запуск другой вакансии: %+v", run.VacancyID)
		}
	}
}

func TestListLLMRunsPagination(t *testing.T) {
	db := newMemoryDB(t)

	for i := 0; i < 5; i++ {
		run := makeRun(model.RunStatusOK, nil, 100, 10, nil)
		if err := CreateLLMRun(db, &run); err != nil {
			t.Fatalf("создание записи: %v", err)
		}
	}

	page, count, err := ListLLMRuns(db, LLMRunFilter{Limit: 2, Offset: 1})
	if err != nil {
		t.Fatalf("ListLLMRuns: %v", err)
	}

	// count — сколько всего подходит, а не сколько отдано.
	if count != 5 {
		t.Errorf("count = %d, ожидалось 5", count)
	}
	if len(page) != 2 {
		t.Errorf("отдано %d записей, ожидалось 2", len(page))
	}
}

func TestLLMRunFilterNormalize(t *testing.T) {
	cases := []struct {
		name  string
		given LLMRunFilter
		want  int
	}{
		{name: "по умолчанию", given: LLMRunFilter{}, want: DefaultRunsLimit},
		{name: "ноль", given: LLMRunFilter{Limit: 0}, want: DefaultRunsLimit},
		{name: "отрицательный", given: LLMRunFilter{Limit: -10}, want: DefaultRunsLimit},
		{name: "слишком большой", given: LLMRunFilter{Limit: 10000}, want: MaxRunsLimit},
		{name: "разумный", given: LLMRunFilter{Limit: 20}, want: 20},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.given.Normalize().Limit; got != tc.want {
				t.Errorf("Limit = %d, ожидалось %d", got, tc.want)
			}
		})
	}

	if got := (LLMRunFilter{Offset: -5}).Normalize().Offset; got != 0 {
		t.Errorf("Offset = %d, ожидался 0", got)
	}
}

func TestLLMRunsUsage(t *testing.T) {
	db := newMemoryDB(t)

	firstCost := 0.003
	secondCost := 0.007

	runs := []model.LLMRun{
		makeRun(model.RunStatusOK, nil, 1000, 100, &firstCost),
		makeRun(model.RunStatusOK, nil, 2000, 200, &secondCost),
		// Запуск без прайса: токены считаются, стоимость — нет.
		makeRun(model.RunStatusOK, nil, 500, 50, nil),
		// Ошибка скачивания: токенов не было вовсе.
		makeRun(model.RunStatusFetchError, nil, 0, 0, nil),
	}
	for i := range runs {
		if err := CreateLLMRun(db, &runs[i]); err != nil {
			t.Fatalf("создание записи: %v", err)
		}
	}

	usage, err := LLMRunsUsage(db)
	if err != nil {
		t.Fatalf("LLMRunsUsage: %v", err)
	}

	if usage.Runs != 4 {
		t.Errorf("Runs = %d, ожидалось 4", usage.Runs)
	}
	if usage.InputTokens != 3500 {
		t.Errorf("InputTokens = %d, ожидалось 3500", usage.InputTokens)
	}
	if usage.OutputTokens != 350 {
		t.Errorf("OutputTokens = %d, ожидалось 350", usage.OutputTokens)
	}
	if usage.CostEstimate == nil {
		t.Fatal("CostEstimate = nil")
	}
	if diff := *usage.CostEstimate - 0.010; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("CostEstimate = %v, ожидалось 0.010", *usage.CostEstimate)
	}
	// Видно, что сумма собрана не по всем запускам.
	if usage.PricedRuns != 2 {
		t.Errorf("PricedRuns = %d, ожидалось 2", usage.PricedRuns)
	}
}

func TestLLMRunsUsageEmptyJournal(t *testing.T) {
	db := newMemoryDB(t)

	usage, err := LLMRunsUsage(db)
	if err != nil {
		t.Fatalf("LLMRunsUsage: %v", err)
	}

	if usage.Runs != 0 || usage.InputTokens != 0 || usage.OutputTokens != 0 {
		t.Errorf("пустой журнал дал %+v", usage)
	}
	// Без запусков с прайсом суммы нет: ноль был бы обманом.
	if usage.CostEstimate != nil {
		t.Errorf("CostEstimate = %v, ожидался nil", *usage.CostEstimate)
	}
}
