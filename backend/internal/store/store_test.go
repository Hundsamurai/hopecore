package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

// newMemoryDB поднимает миграцию на чистой in-memory БД.
func newMemoryDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := OpenMemory(nil)
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() {
		if err := Close(db); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return db
}

func TestMigrateCreatesAllTables(t *testing.T) {
	db := newMemoryDB(t)

	for _, table := range TableNames() {
		if !db.Migrator().HasTable(table) {
			t.Errorf("таблица %q не создана", table)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	db := newMemoryDB(t)

	// Повторный запуск на каждом старте контейнера не должен ничего ломать.
	if err := Migrate(db); err != nil {
		t.Fatalf("повторная миграция: %v", err)
	}
	for _, table := range TableNames() {
		if !db.Migrator().HasTable(table) {
			t.Errorf("таблица %q пропала после повторной миграции", table)
		}
	}
}

func TestForeignKeysPragmaEnabled(t *testing.T) {
	db := newMemoryDB(t)

	fk, err := PragmaInt(db, "foreign_keys")
	if err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Fatalf("foreign_keys = %d, ожидалось 1 (иначе каскадное удаление не работает)", fk)
	}
}

func TestOpenFileUsesRollbackJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hopecore.db")

	db, err := Open(path, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() {
		if err := Close(db); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	if err := Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	mode, err := PragmaString(db, "journal_mode")
	if err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	// WAL сознательно не используется: файл лежит в bind-mount и доступен с хоста,
	// а разделяемая память WAL на такой связке однажды привела к повреждению БД.
	if !strings.EqualFold(mode, "delete") {
		t.Errorf("journal_mode = %q, ожидался delete", mode)
	}

	// Файла -shm быть не должно: это признак включённого WAL.
	if _, err := os.Stat(path + "-shm"); err == nil {
		t.Error("рядом с БД появился файл -shm, значит включён WAL")
	}

	fk, err := PragmaInt(db, "foreign_keys")
	if err != nil {
		t.Fatalf("PRAGMA foreign_keys: %v", err)
	}
	if fk != 1 {
		t.Errorf("foreign_keys = %d, ожидалось 1", fk)
	}
}

func TestOpenCreatesMissingDirectory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "hopecore.db")

	db, err := Open(path, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := Close(db); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestVacancyRoundTrip(t *testing.T) {
	db := newMemoryDB(t)

	opened := model.NewDate(2026, time.August, 1)
	auto := true
	code := 200
	checkedAt := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)

	want := model.Vacancy{
		URL:            "https://example.com/vacancy/1",
		Company:        "Example",
		Grade:          model.GradeSenior,
		TechTags:       model.Tags{"go", "postgres", "docker"},
		OpenedDate:     &opened,
		AutoIsActive:   &auto,
		LastCheckedAt:  &checkedAt,
		LastCheckCode:  &code,
		LastCheckError: "",
	}

	if err := db.Create(&want).Error; err != nil {
		t.Fatalf("создание вакансии: %v", err)
	}
	if want.ID == 0 {
		t.Fatal("id не присвоен")
	}
	if want.CreatedAt.IsZero() || want.UpdatedAt.IsZero() {
		t.Error("created_at/updated_at не заполнены")
	}

	var got model.Vacancy
	if err := db.First(&got, want.ID).Error; err != nil {
		t.Fatalf("чтение вакансии: %v", err)
	}

	if got.URL != want.URL || got.Company != want.Company || got.Grade != want.Grade {
		t.Errorf("текстовые поля не совпали: %+v", got)
	}
	if got.OpenedDate == nil || got.OpenedDate.String() != "2026-08-01" {
		t.Errorf("opened_date = %v, ожидалось 2026-08-01", got.OpenedDate)
	}
	if got.AutoIsActive == nil || !*got.AutoIsActive {
		t.Errorf("auto_is_active = %v, ожидалось true", got.AutoIsActive)
	}
	if got.ManualIsActive != nil {
		t.Errorf("manual_is_active = %v, ожидался nil (override не задавали)", *got.ManualIsActive)
	}
	if got.LastCheckCode == nil || *got.LastCheckCode != 200 {
		t.Errorf("last_check_code = %v, ожидалось 200", got.LastCheckCode)
	}
	if got.LastCheckedAt == nil || !got.LastCheckedAt.Equal(checkedAt) {
		t.Errorf("last_checked_at = %v, ожидалось %v", got.LastCheckedAt, checkedAt)
	}
}

func TestCandidateStatusUniquePerVacancy(t *testing.T) {
	db := newMemoryDB(t)

	vacancy := model.Vacancy{URL: "https://example.com/vacancy/2"}
	if err := db.Create(&vacancy).Error; err != nil {
		t.Fatalf("создание вакансии: %v", err)
	}

	first := model.CandidateStatus{VacancyID: vacancy.ID, InterviewStage: "отклик отправлен"}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("создание статуса: %v", err)
	}

	// Связь 1:1 — вторая запись по той же вакансии должна упереться в уникальный индекс.
	second := model.CandidateStatus{VacancyID: vacancy.ID}
	err := db.Create(&second).Error
	if err == nil {
		t.Fatal("второй статус по той же вакансии создан, ожидалась ошибка уникальности")
	}
	if !errors.Is(err, gorm.ErrDuplicatedKey) {
		t.Logf("получена ошибка (тип не gorm.ErrDuplicatedKey, но вставка отклонена): %v", err)
	}
}

func TestDeleteVacancyCascadesStatusAndSummary(t *testing.T) {
	db := newMemoryDB(t)

	vacancy := model.Vacancy{URL: "https://example.com/vacancy/3"}
	if err := db.Create(&vacancy).Error; err != nil {
		t.Fatalf("создание вакансии: %v", err)
	}

	status := model.CandidateStatus{VacancyID: vacancy.ID}
	if err := db.Create(&status).Error; err != nil {
		t.Fatalf("создание статуса: %v", err)
	}

	summary := model.InterviewSummary{CandidateStatusID: status.ID, AIScore: "7/10"}
	if err := db.Create(&summary).Error; err != nil {
		t.Fatalf("создание резюме собеса: %v", err)
	}

	aiBlock := model.AIBlock{VacancyID: vacancy.ID, SalaryForecast: "300k"}
	if err := db.Create(&aiBlock).Error; err != nil {
		t.Fatalf("создание нейроблока: %v", err)
	}

	if err := db.Delete(&model.Vacancy{}, vacancy.ID).Error; err != nil {
		t.Fatalf("удаление вакансии: %v", err)
	}

	assertCount(t, db, &model.CandidateStatus{}, 0, "статус кандидата")
	assertCount(t, db, &model.InterviewSummary{}, 0, "резюме собеседования")
	assertCount(t, db, &model.AIBlock{}, 0, "нейроблок")
}

func TestCandidateStatusRequiresExistingVacancy(t *testing.T) {
	db := newMemoryDB(t)

	// Внешний ключ включён, значит ссылка на несуществующую вакансию недопустима.
	err := db.Create(&model.CandidateStatus{VacancyID: 999}).Error
	if err == nil {
		t.Fatal("статус с несуществующим vacancy_id создан, ожидалась ошибка внешнего ключа")
	}
}

func assertCount(t *testing.T, db *gorm.DB, dest any, want int64, name string) {
	t.Helper()

	var got int64
	if err := db.Model(dest).Count(&got).Error; err != nil {
		t.Fatalf("подсчёт %s: %v", name, err)
	}
	if got != want {
		t.Errorf("%s: осталось %d записей, ожидалось %d", name, got, want)
	}
}

func TestOpenCorruptedFileGivesReadableError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hopecore.db")

	// Ровно то, что мы однажды получили в bind-mount: файл нужного размера из нулей.
	if err := os.WriteFile(path, make([]byte, 4096), 0o644); err != nil {
		t.Fatalf("подготовка файла: %v", err)
	}

	db, err := Open(path, nil)
	if err == nil {
		_ = Close(db)
		t.Fatal("Open принял испорченный файл, ожидалась ошибка")
	}

	// Сообщение должно объяснять, что делать, а не только цитировать SQLite.
	for _, fragment := range []string{"не является базой SQLite", "integrity_check"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("в сообщении нет %q: %v", fragment, err)
		}
	}
}

func TestLLMRunSurvivesVacancyDeletion(t *testing.T) {
	db := newMemoryDB(t)

	vacancy := model.Vacancy{URL: "https://example.com/vacancy/llm"}
	if err := db.Create(&vacancy).Error; err != nil {
		t.Fatalf("создание вакансии: %v", err)
	}

	tokens := 1500
	run := model.LLMRun{
		Purpose:       model.PurposeExtractVacancy,
		VacancyID:     &vacancy.ID,
		Provider:      "gemini",
		Model:         "gemini-2.5-flash",
		PromptVersion: "extract-v1",
		SourceURL:     vacancy.URL,
		SourceChars:   2900,
		Status:        model.RunStatusOK,
		InputTokens:   &tokens,
		Attempts:      1,
		DurationMs:    1200,
		ResponseJSON:  `{"title":"Go-разработчик"}`,
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("создание записи журнала: %v", err)
	}

	if err := db.Delete(&model.Vacancy{}, vacancy.ID).Error; err != nil {
		t.Fatalf("удаление вакансии: %v", err)
	}

	// История трат не должна исчезать вместе с карточкой: внешний ключ
	// обнуляется, а запись остаётся.
	var stored model.LLMRun
	if err := db.First(&stored, run.ID).Error; err != nil {
		t.Fatalf("запись журнала пропала после удаления вакансии: %v", err)
	}
	if stored.VacancyID != nil {
		t.Errorf("vacancy_id = %v, ожидался nil", *stored.VacancyID)
	}
	if stored.Provider != "gemini" || stored.Status != model.RunStatusOK {
		t.Errorf("данные записи испортились: %+v", stored)
	}
	if stored.InputTokens == nil || *stored.InputTokens != 1500 {
		t.Errorf("input_tokens = %v", stored.InputTokens)
	}
}

func TestLLMRunNullableCounters(t *testing.T) {
	db := newMemoryDB(t)

	// Провайдер может не вернуть счётчики, и ноль был бы неправдой.
	run := model.LLMRun{
		Purpose:  model.PurposeExtractVacancy,
		Provider: "deepseek",
		Model:    "deepseek-chat",
		Status:   model.RunStatusFetchError,
		Error:    "сайт ответил 403",
	}
	if err := db.Create(&run).Error; err != nil {
		t.Fatalf("создание записи: %v", err)
	}

	var stored model.LLMRun
	if err := db.First(&stored, run.ID).Error; err != nil {
		t.Fatalf("чтение записи: %v", err)
	}
	if stored.InputTokens != nil || stored.OutputTokens != nil || stored.CostEstimate != nil {
		t.Errorf("счётчики не nil: %v / %v / %v", stored.InputTokens, stored.OutputTokens, stored.CostEstimate)
	}
	// Запись об ошибке скачивания тоже попадает в журнал: видно, что попытка была.
	if stored.VacancyID != nil {
		t.Errorf("vacancy_id = %v", *stored.VacancyID)
	}
}
