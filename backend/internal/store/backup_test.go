package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

// createVacancy добавляет вакансию с заметным заголовком, чтобы по нему
// отличать состояния базы до и после восстановления.
func createVacancy(t *testing.T, db *gorm.DB, title string) uint {
	t.Helper()

	vacancy := model.Vacancy{URL: "https://example.com/" + title, Title: title}
	if err := CreateVacancy(db, &vacancy); err != nil {
		t.Fatalf("создание вакансии %q: %v", title, err)
	}
	return vacancy.ID
}

// vacancyTitles возвращает заголовки всех вакансий по возрастанию id.
func vacancyTitles(t *testing.T, db *gorm.DB) []string {
	t.Helper()

	var titles []string
	if err := db.Raw(`SELECT title FROM vacancies ORDER BY id`).Scan(&titles).Error; err != nil {
		t.Fatalf("чтение заголовков: %v", err)
	}
	return titles
}

func TestBackupDirIsNextToDatabase(t *testing.T) {
	if got, want := BackupDir("/data/hopecore.db"), filepath.Join("/data", "backups"); got != want {
		t.Errorf("BackupDir = %q, ожидалось %q", got, want)
	}
}

func TestCreateBackupWritesFile(t *testing.T) {
	db := newMemoryDB(t)
	dir := filepath.Join(t.TempDir(), "backups")

	createVacancy(t, db, "го-разработчик")

	backup, err := CreateBackup(db, dir, "")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	if !backupNamePattern.MatchString(backup.Name) {
		t.Errorf("имя %q не соответствует ожидаемому формату", backup.Name)
	}
	if backup.Automatic {
		t.Error("копия по кнопке не должна помечаться как автоматическая")
	}
	if backup.SizeBytes <= 0 {
		t.Errorf("размер копии = %d, ожидался положительный", backup.SizeBytes)
	}

	info, err := os.Stat(filepath.Join(dir, backup.Name))
	if err != nil {
		t.Fatalf("файл копии не создан: %v", err)
	}
	if info.Size() != backup.SizeBytes {
		t.Errorf("размер в ответе %d, на диске %d", backup.SizeBytes, info.Size())
	}
}

// Две копии подряд попадают в одну секунду, а VACUUM INTO отказывается писать
// в существующий файл — имена обязаны различаться.
func TestCreateBackupTwiceInSameSecond(t *testing.T) {
	db := newMemoryDB(t)
	dir := t.TempDir()

	first, err := CreateBackup(db, dir, "")
	if err != nil {
		t.Fatalf("первая копия: %v", err)
	}
	second, err := CreateBackup(db, dir, "")
	if err != nil {
		t.Fatalf("вторая копия: %v", err)
	}

	if first.Name == second.Name {
		t.Fatalf("обе копии получили одно имя %q", first.Name)
	}
	for _, name := range []string{first.Name, second.Name} {
		if !backupNamePattern.MatchString(name) {
			t.Errorf("имя %q не соответствует ожидаемому формату", name)
		}
	}
}

func TestListBackupsSortsNewestFirstAndIgnoresForeignFiles(t *testing.T) {
	db := newMemoryDB(t)
	dir := t.TempDir()

	first, err := CreateBackup(db, dir, "")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	second, err := CreateBackup(db, dir, SuffixBeforeRestore)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Посторонние файлы в каталоге не должны попадать в список.
	for _, name := range []string{"README.md", "hopecore.db", "заметка.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("не копия"), 0o644); err != nil {
			t.Fatalf("подготовка файла %s: %v", name, err)
		}
	}

	backups, err := ListBackups(dir)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 2 {
		names := make([]string, 0, len(backups))
		for _, b := range backups {
			names = append(names, b.Name)
		}
		t.Fatalf("получено %d копий (%v), ожидалось 2", len(backups), names)
	}

	// Обе копии созданы в одну секунду, поэтому порядок задаётся именем:
	// у второй есть суффикс, и она должна оказаться выше.
	if backups[0].Name != second.Name || backups[1].Name != first.Name {
		t.Errorf("порядок = [%s %s], ожидался [%s %s]",
			backups[0].Name, backups[1].Name, second.Name, first.Name)
	}
	if !backups[0].Automatic {
		t.Errorf("копия %q снята перед восстановлением и должна быть автоматической", backups[0].Name)
	}
	if backups[1].Automatic {
		t.Errorf("копия %q снята по кнопке и не должна быть автоматической", backups[1].Name)
	}
}

// Каталога копий может не быть: до первой копии его никто не создаёт.
func TestListBackupsOnMissingDirectory(t *testing.T) {
	backups, err := ListBackups(filepath.Join(t.TempDir(), "нет-такого"))
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("получено %d копий, ожидался пустой список", len(backups))
	}
}

func TestRestoreBackupReturnsData(t *testing.T) {
	db := newMemoryDB(t)
	dir := t.TempDir()

	createVacancy(t, db, "было")

	backup, err := CreateBackup(db, dir, "")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Портим данные так, как это сделала бы ошибка пользователя.
	if err := db.Exec(`DELETE FROM vacancies`).Error; err != nil {
		t.Fatalf("удаление вакансий: %v", err)
	}
	createVacancy(t, db, "мусор")

	safety, err := RestoreBackup(db, dir, backup.Name)
	if err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}

	if got, want := vacancyTitles(t, db), []string{"было"}; !equalStrings(got, want) {
		t.Errorf("после восстановления заголовки = %v, ожидались %v", got, want)
	}

	// Копия состояния до восстановления обязана лежать на диске: без неё
	// ошибочное восстановление было бы необратимым.
	if !strings.Contains(safety.Name, SuffixBeforeRestore) {
		t.Errorf("имя страховочной копии %q не содержит %q", safety.Name, SuffixBeforeRestore)
	}
	if _, err := os.Stat(filepath.Join(dir, safety.Name)); err != nil {
		t.Fatalf("страховочная копия не создана: %v", err)
	}
	if !safety.Automatic {
		t.Error("страховочная копия должна быть помечена как автоматическая")
	}
}

// Главный сценарий задачи: восстановились не туда — откатываемся обратно.
func TestRestoreIsReversibleViaSafetyBackup(t *testing.T) {
	db := newMemoryDB(t)
	dir := t.TempDir()

	createVacancy(t, db, "старое-состояние")
	old, err := CreateBackup(db, dir, "")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	if err := db.Exec(`DELETE FROM vacancies`).Error; err != nil {
		t.Fatalf("удаление вакансий: %v", err)
	}
	createVacancy(t, db, "текущее-состояние")

	// Восстановились по ошибке.
	safety, err := RestoreBackup(db, dir, old.Name)
	if err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	if got, want := vacancyTitles(t, db), []string{"старое-состояние"}; !equalStrings(got, want) {
		t.Fatalf("после восстановления заголовки = %v, ожидались %v", got, want)
	}

	// Отменяем: страховочная копия возвращает то, что было до восстановления.
	if _, err := RestoreBackup(db, dir, safety.Name); err != nil {
		t.Fatalf("восстановление из страховочной копии: %v", err)
	}
	if got, want := vacancyTitles(t, db), []string{"текущее-состояние"}; !equalStrings(got, want) {
		t.Errorf("после отмены заголовки = %v, ожидались %v", got, want)
	}
}

// Восстановление затрагивает все таблицы схемы, а не только вакансии.
func TestRestoreBackupCoversAllTables(t *testing.T) {
	db := newMemoryDB(t)
	dir := t.TempDir()

	vacancyID := createVacancy(t, db, "с-историей")
	status := model.CandidateStatus{VacancyID: vacancyID, InterviewStage: "отклик"}
	if err := UpsertCandidateStatus(db, &status); err != nil {
		t.Fatalf("создание статуса кандидата: %v", err)
	}
	run := model.LLMRun{
		Purpose:  "extract",
		Provider: "gemini",
		Model:    "gemini-2.5-flash",
		Status:   "success",
	}
	if err := CreateLLMRun(db, &run); err != nil {
		t.Fatalf("создание запуска модели: %v", err)
	}

	backup, err := CreateBackup(db, dir, "")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Чистим всё, что было записано.
	if err := db.Exec(`DELETE FROM llm_runs`).Error; err != nil {
		t.Fatalf("очистка llm_runs: %v", err)
	}
	if err := DeleteVacancy(db, vacancyID); err != nil {
		t.Fatalf("удаление вакансии: %v", err)
	}

	if _, err := RestoreBackup(db, dir, backup.Name); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}

	if _, err := GetCandidateStatus(db, vacancyID); err != nil {
		t.Errorf("статус кандидата не восстановлен: %v", err)
	}
	if got := countRows(t, db, "llm_runs"); got != 1 {
		t.Errorf("в llm_runs %d записей, ожидалась 1", got)
	}
}

// Копия, снятая старой версией приложения, может не содержать таблицу llm_runs.
// Восстановление обязано отработать и привести базу к состоянию снимка.
func TestRestoreBackupFromOlderSchema(t *testing.T) {
	db := newMemoryDB(t)
	dir := t.TempDir()

	createVacancy(t, db, "из-старой-схемы")
	backup, err := CreateBackup(db, dir, "")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Имитируем старую схему: в копии таблицы запусков ещё нет.
	dropTableInFile(t, filepath.Join(dir, backup.Name), "llm_runs")

	run := model.LLMRun{Purpose: "extract", Provider: "gemini", Model: "m", Status: "success"}
	if err := CreateLLMRun(db, &run); err != nil {
		t.Fatalf("создание запуска модели: %v", err)
	}

	if _, err := RestoreBackup(db, dir, backup.Name); err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}

	if got, want := vacancyTitles(t, db), []string{"из-старой-схемы"}; !equalStrings(got, want) {
		t.Errorf("заголовки = %v, ожидались %v", got, want)
	}
	// В снимке записей о запусках не было, значит и после восстановления их быть не должно.
	if got := countRows(t, db, "llm_runs"); got != 0 {
		t.Errorf("в llm_runs %d записей, ожидалось 0", got)
	}
}

// Имена приходят из HTTP-запроса, поэтому обход каталога должен отсекаться.
func TestBackupNameValidation(t *testing.T) {
	dir := t.TempDir()

	names := []string{
		"",
		"..",
		"../../etc/passwd",
		"../hopecore.db",
		"subdir/hopecore-20260810-120000.db",
		`..\hopecore-20260810-120000.db`,
		"hopecore.db",
		"hopecore-2026-08-10.db",
		"hopecore-20260810-120000.sqlite",
		"hopecore-20260810-120000.db.bak",
		"HOPECORE-20260810-120000.db",
		"hopecore-20260810-120000.db ",
		"hopecore-20260810-1200.db",
	}

	for _, name := range names {
		t.Run("имя="+name, func(t *testing.T) {
			if _, err := backupPath(dir, name); !errors.Is(err, ErrInvalidBackupName) {
				t.Errorf("backupPath(%q) = %v, ожидалась ErrInvalidBackupName", name, err)
			}
			if err := DeleteBackup(dir, name); !errors.Is(err, ErrInvalidBackupName) {
				t.Errorf("DeleteBackup(%q) = %v, ожидалась ErrInvalidBackupName", name, err)
			}
		})
	}
}

// Проверка не только формы имени, но и последствий: файл вне каталога копий
// должен остаться на месте.
func TestDeleteBackupCannotEscapeDirectory(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}

	victim := filepath.Join(root, "hopecore.db")
	if err := os.WriteFile(victim, []byte("рабочая база"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := DeleteBackup(dir, "../hopecore.db"); !errors.Is(err, ErrInvalidBackupName) {
		t.Errorf("DeleteBackup = %v, ожидалась ErrInvalidBackupName", err)
	}
	if _, err := os.Stat(victim); err != nil {
		t.Errorf("файл вне каталога копий пострадал: %v", err)
	}
}

func TestBackupOperationsOnMissingFile(t *testing.T) {
	db := newMemoryDB(t)
	dir := t.TempDir()
	missing := "hopecore-20260810-120000.db"

	if err := DeleteBackup(dir, missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("DeleteBackup = %v, ожидалась ErrNotFound", err)
	}
	if _, err := RestoreBackup(db, dir, missing); !errors.Is(err, ErrNotFound) {
		t.Errorf("RestoreBackup = %v, ожидалась ErrNotFound", err)
	}
}

func TestDeleteBackupRemovesFile(t *testing.T) {
	db := newMemoryDB(t)
	dir := t.TempDir()

	backup, err := CreateBackup(db, dir, "")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if err := DeleteBackup(dir, backup.Name); err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dir, backup.Name)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("файл копии остался на диске: %v", err)
	}
	backups, err := ListBackups(dir)
	if err != nil {
		t.Fatalf("ListBackups: %v", err)
	}
	if len(backups) != 0 {
		t.Errorf("в списке %d копий, ожидался пустой список", len(backups))
	}
}

// Посторонний файл с правильным именем не должен уничтожить рабочие данные.
func TestRestoreRejectsFileThatIsNotDatabase(t *testing.T) {
	db := newMemoryDB(t)
	dir := t.TempDir()

	createVacancy(t, db, "рабочие-данные")

	fake := filepath.Join(dir, "hopecore-20260810-120000.db")
	if err := os.WriteFile(fake, []byte("это вообще не база данных"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := RestoreBackup(db, dir, "hopecore-20260810-120000.db")
	if !errors.Is(err, ErrNotABackup) {
		t.Fatalf("RestoreBackup = %v, ожидалась ErrNotABackup", err)
	}

	// Данные не тронуты.
	if got, want := vacancyTitles(t, db), []string{"рабочие-данные"}; !equalStrings(got, want) {
		t.Errorf("заголовки = %v, ожидались %v", got, want)
	}
}

// База SQLite, но чужая: без таблицы vacancies восстанавливать нечего.
func TestRestoreRejectsForeignDatabase(t *testing.T) {
	db := newMemoryDB(t)
	dir := t.TempDir()

	createVacancy(t, db, "рабочие-данные")

	foreign := filepath.Join(dir, "hopecore-20260810-121500.db")
	other, err := Open(foreign, nil)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := other.Exec(`CREATE TABLE something (id integer primary key)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := Close(other); err != nil {
		t.Fatal(err)
	}

	if _, err := RestoreBackup(db, dir, "hopecore-20260810-121500.db"); !errors.Is(err, ErrNotABackup) {
		t.Fatalf("RestoreBackup = %v, ожидалась ErrNotABackup", err)
	}
	if got, want := vacancyTitles(t, db), []string{"рабочие-данные"}; !equalStrings(got, want) {
		t.Errorf("заголовки = %v, ожидались %v", got, want)
	}
}

// После восстановления соединение должно остаться пригодным: лишняя
// подключённая база сломала бы последующие запросы и повторное восстановление.
func TestRestoreDetachesSourceDatabase(t *testing.T) {
	db := newMemoryDB(t)
	dir := t.TempDir()

	createVacancy(t, db, "первая")
	backup, err := CreateBackup(db, dir, "")
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	for i := 0; i < 3; i++ {
		if _, err := RestoreBackup(db, dir, backup.Name); err != nil {
			t.Fatalf("восстановление №%d: %v", i+1, err)
		}
	}

	if got, want := vacancyTitles(t, db), []string{"первая"}; !equalStrings(got, want) {
		t.Errorf("заголовки = %v, ожидались %v", got, want)
	}
}

func countRows(t *testing.T, db *gorm.DB, table string) int64 {
	t.Helper()

	var count int64
	if err := db.Raw(`SELECT count(*) FROM ` + quoteIdent(table)).Scan(&count).Error; err != nil {
		t.Fatalf("подсчёт строк в %s: %v", table, err)
	}
	return count
}

// dropTableInFile удаляет таблицу в отдельном файле базы, имитируя копию,
// снятую версией приложения с другой схемой.
func dropTableInFile(t *testing.T, path, table string) {
	t.Helper()

	db, err := Open(path, nil)
	if err != nil {
		t.Fatalf("открытие %s: %v", path, err)
	}
	if err := db.Exec(`DROP TABLE ` + quoteIdent(table)).Error; err != nil {
		t.Fatalf("удаление таблицы %s: %v", table, err)
	}
	if err := Close(db); err != nil {
		t.Fatalf("закрытие %s: %v", path, err)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
