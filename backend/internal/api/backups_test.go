package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
	"github.com/Hundsamurai/hopecore/backend/internal/store"
)

// createVacancyViaAPI добавляет вакансию и возвращает её id.
func (e *testEnv) createVacancyViaAPI(title string) uint {
	e.t.Helper()

	rec := e.request(http.MethodPost, "/api/vacancies", map[string]any{
		"url":   "https://example.com/" + title,
		"title": title,
	})
	var created vacancyResponse
	e.decode(rec, http.StatusCreated, &created)
	return created.ID
}

// vacancyTitles возвращает заголовки вакансий, как их видит API.
func (e *testEnv) vacancyTitles() []string {
	e.t.Helper()

	rec := e.request(http.MethodGet, "/api/vacancies?active=all", nil)
	var list listVacanciesResponse
	e.decode(rec, http.StatusOK, &list)

	titles := make([]string, 0, len(list.Items))
	for _, v := range list.Items {
		titles = append(titles, v.Title)
	}
	return titles
}

func TestListBackupsEmpty(t *testing.T) {
	env := newTestEnv(t)

	rec := env.request(http.MethodGet, "/api/backups", nil)
	var body backupsResponse
	env.decode(rec, http.StatusOK, &body)

	// Пустой список, а не null: фронтенд не должен разбирать два случая.
	if body.Items == nil {
		t.Error("items = null, ожидался пустой массив")
	}
	if len(body.Items) != 0 {
		t.Errorf("получено %d копий, ожидался пустой список", len(body.Items))
	}
	if body.TotalBytes != 0 {
		t.Errorf("total_bytes = %d, ожидался 0", body.TotalBytes)
	}
	if body.Dir != env.backupDir {
		t.Errorf("dir = %q, ожидался %q", body.Dir, env.backupDir)
	}
}

func TestCreateBackupThenList(t *testing.T) {
	env := newTestEnv(t)
	env.createVacancyViaAPI("го-разработчик")

	rec := env.request(http.MethodPost, "/api/backups", nil)
	var created backupResponse
	env.decode(rec, http.StatusCreated, &created)

	if created.Name == "" {
		t.Fatal("имя копии пустое")
	}
	if created.SizeBytes <= 0 {
		t.Errorf("size_bytes = %d, ожидался положительный", created.SizeBytes)
	}
	if created.CreatedAt.IsZero() {
		t.Error("created_at не заполнено")
	}
	if created.Automatic {
		t.Error("копия по кнопке не должна быть помечена автоматической")
	}

	listRec := env.request(http.MethodGet, "/api/backups", nil)
	var body backupsResponse
	env.decode(listRec, http.StatusOK, &body)

	if len(body.Items) != 1 {
		t.Fatalf("получено %d копий, ожидалась 1", len(body.Items))
	}
	if body.Items[0].Name != created.Name {
		t.Errorf("в списке %q, создавали %q", body.Items[0].Name, created.Name)
	}
	if body.TotalBytes != created.SizeBytes {
		t.Errorf("total_bytes = %d, размер копии %d", body.TotalBytes, created.SizeBytes)
	}
}

func TestRestoreBackupViaAPI(t *testing.T) {
	env := newTestEnv(t)
	env.createVacancyViaAPI("было")

	rec := env.request(http.MethodPost, "/api/backups", nil)
	var created backupResponse
	env.decode(rec, http.StatusCreated, &created)

	// Портим данные: удаляем прежнюю вакансию и добавляем новую.
	env.createVacancyViaAPI("мусор")
	if err := env.db.Exec(`DELETE FROM vacancies WHERE title = 'было'`).Error; err != nil {
		t.Fatalf("удаление вакансии: %v", err)
	}

	restoreRec := env.request(http.MethodPost, "/api/backups/"+created.Name+"/restore", nil)
	var restored restoreResponse
	env.decode(restoreRec, http.StatusOK, &restored)

	if restored.Restored != created.Name {
		t.Errorf("restored = %q, ожидалось %q", restored.Restored, created.Name)
	}
	if !strings.Contains(restored.SafetyBackup, store.SuffixBeforeRestore) {
		t.Errorf("safety_backup = %q, ожидалась пометка %q",
			restored.SafetyBackup, store.SuffixBeforeRestore)
	}
	if _, err := os.Stat(filepath.Join(env.backupDir, restored.SafetyBackup)); err != nil {
		t.Errorf("страховочная копия не создана: %v", err)
	}

	if got, want := env.vacancyTitles(), []string{"было"}; !equalTitles(got, want) {
		t.Errorf("после восстановления заголовки = %v, ожидались %v", got, want)
	}

	// Страховочная копия видна в списке и помечена как автоматическая.
	listRec := env.request(http.MethodGet, "/api/backups", nil)
	var body backupsResponse
	env.decode(listRec, http.StatusOK, &body)

	var found bool
	for _, item := range body.Items {
		if item.Name != restored.SafetyBackup {
			continue
		}
		found = true
		if !item.Automatic {
			t.Error("страховочная копия должна быть помечена автоматической")
		}
	}
	if !found {
		t.Errorf("страховочной копии %q нет в списке", restored.SafetyBackup)
	}
}

// Восстановление из страховочной копии отменяет ошибочное восстановление.
func TestRestoreIsReversibleViaAPI(t *testing.T) {
	env := newTestEnv(t)
	env.createVacancyViaAPI("старое")

	rec := env.request(http.MethodPost, "/api/backups", nil)
	var old backupResponse
	env.decode(rec, http.StatusCreated, &old)

	if err := env.db.Exec(`DELETE FROM vacancies`).Error; err != nil {
		t.Fatalf("очистка вакансий: %v", err)
	}
	env.createVacancyViaAPI("текущее")

	restoreRec := env.request(http.MethodPost, "/api/backups/"+old.Name+"/restore", nil)
	var restored restoreResponse
	env.decode(restoreRec, http.StatusOK, &restored)

	if got, want := env.vacancyTitles(), []string{"старое"}; !equalTitles(got, want) {
		t.Fatalf("заголовки = %v, ожидались %v", got, want)
	}

	undoRec := env.request(http.MethodPost, "/api/backups/"+restored.SafetyBackup+"/restore", nil)
	env.decode(undoRec, http.StatusOK, nil)

	if got, want := env.vacancyTitles(), []string{"текущее"}; !equalTitles(got, want) {
		t.Errorf("после отмены заголовки = %v, ожидались %v", got, want)
	}
}

func TestDeleteBackupViaAPI(t *testing.T) {
	env := newTestEnv(t)

	rec := env.request(http.MethodPost, "/api/backups", nil)
	var created backupResponse
	env.decode(rec, http.StatusCreated, &created)

	delRec := env.request(http.MethodDelete, "/api/backups/"+created.Name, nil)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("статус = %d, ожидался 204, тело: %s", delRec.Code, delRec.Body.String())
	}
	if delRec.Body.Len() != 0 {
		t.Errorf("тело ответа не пустое: %s", delRec.Body.String())
	}

	listRec := env.request(http.MethodGet, "/api/backups", nil)
	var body backupsResponse
	env.decode(listRec, http.StatusOK, &body)
	if len(body.Items) != 0 {
		t.Errorf("после удаления в списке %d копий", len(body.Items))
	}
}

// Имя копии приходит из URL, поэтому обход каталога обязан отсекаться на входе.
func TestBackupBadNameReturns400(t *testing.T) {
	env := newTestEnv(t)

	names := []string{
		"hopecore.db",
		"hopecore-2026.db",
		"%2e%2e%2fhopecore.db",
		"..%2f..%2fetc%2fpasswd",
	}

	for _, name := range names {
		t.Run("имя="+name, func(t *testing.T) {
			restoreRec := env.request(http.MethodPost, "/api/backups/"+name+"/restore", nil)
			payload := env.decodeError(restoreRec, http.StatusBadRequest)
			if payload.Code != CodeValidationFailed {
				t.Errorf("code = %q, ожидался %q", payload.Code, CodeValidationFailed)
			}

			delRec := env.request(http.MethodDelete, "/api/backups/"+name, nil)
			env.decodeError(delRec, http.StatusBadRequest)
		})
	}
}

func TestBackupMissingFileReturns404(t *testing.T) {
	env := newTestEnv(t)
	missing := "hopecore-20260810-120000.db"

	restoreRec := env.request(http.MethodPost, "/api/backups/"+missing+"/restore", nil)
	payload := env.decodeError(restoreRec, http.StatusNotFound)
	if payload.Code != CodeNotFound {
		t.Errorf("code = %q, ожидался %q", payload.Code, CodeNotFound)
	}

	delRec := env.request(http.MethodDelete, "/api/backups/"+missing, nil)
	env.decodeError(delRec, http.StatusNotFound)
}

// Восстановление меняет все данные, поэтому GET-запросом оно происходить не должно.
func TestBackupMethodNotAllowed(t *testing.T) {
	env := newTestEnv(t)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/backups/hopecore-20260810-120000.db/restore"},
		{http.MethodDelete, "/api/backups"},
		{http.MethodPatch, "/api/backups"},
		{http.MethodPut, "/api/backups/hopecore-20260810-120000.db"},
	}

	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			rec := env.request(c.method, c.path, nil)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Errorf("статус = %d, ожидался 405, тело: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// Восстановление должно возвращать не только вакансии, но и связанные таблицы.
func TestRestoreBringsBackCandidateStatus(t *testing.T) {
	env := newTestEnv(t)
	id := env.createVacancyViaAPI("с-откликом")

	statusRec := env.request(http.MethodPut, "/api/vacancies/"+itoa(id)+"/candidate-status", map[string]any{
		"interview_stage": "первое интервью",
	})
	env.decode(statusRec, http.StatusOK, nil)

	rec := env.request(http.MethodPost, "/api/backups", nil)
	var created backupResponse
	env.decode(rec, http.StatusCreated, &created)

	// Удаление вакансии каскадом уносит статус кандидата.
	delRec := env.request(http.MethodDelete, "/api/vacancies/"+itoa(id), nil)
	if delRec.Code != http.StatusNoContent {
		t.Fatalf("удаление вакансии: статус %d", delRec.Code)
	}

	restoreRec := env.request(http.MethodPost, "/api/backups/"+created.Name+"/restore", nil)
	env.decode(restoreRec, http.StatusOK, nil)

	var status model.CandidateStatus
	if err := env.db.Where("vacancy_id = ?", id).First(&status).Error; err != nil {
		t.Fatalf("статус кандидата не восстановлен: %v", err)
	}
	if status.InterviewStage != "первое интервью" {
		t.Errorf("interview_stage = %q, ожидалось %q", status.InterviewStage, "первое интервью")
	}
}

func equalTitles(a, b []string) bool {
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
