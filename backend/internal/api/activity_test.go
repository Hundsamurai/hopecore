package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/Hundsamurai/hopecore/backend/internal/activity"
	"github.com/Hundsamurai/hopecore/backend/internal/model"
	"github.com/Hundsamurai/hopecore/backend/internal/service"
)

func TestCheckOneVacancyMarksInactive(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{"url": "https://example.com/jobs/closed"})
	env.checker.set(created.URL, activity.Result{Active: boolPtr(false), StatusCode: intPtr(404)})

	var checked vacancyResponse
	env.decode(env.request(http.MethodPost, "/api/vacancies/"+itoa(created.ID)+"/check", nil), http.StatusOK, &checked)

	if checked.IsActive {
		t.Error("is_active = true, ожидалось false после 404")
	}
	if checked.AutoIsActive == nil || *checked.AutoIsActive {
		t.Errorf("auto_is_active = %v, ожидалось false", checked.AutoIsActive)
	}
	if checked.LastCheckCode == nil || *checked.LastCheckCode != 404 {
		t.Errorf("last_check_code = %v, ожидалось 404", checked.LastCheckCode)
	}
	if checked.LastCheckedAt == nil {
		t.Error("last_checked_at не заполнен")
	}
	if checked.LastCheckError != "" {
		t.Errorf("last_check_error = %q, ожидалась пустая строка: результат определённый", checked.LastCheckError)
	}
	if checked.ActivityConflict {
		t.Error("activity_conflict = true, хотя ручного override нет")
	}
}

func TestCheckOneVacancyUnknownKeepsPreviousState(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{"url": "https://example.com/jobs/flaky"})

	// Сначала сайт ответил 200.
	env.checker.set(created.URL, activity.Result{Active: boolPtr(true), StatusCode: intPtr(200)})
	env.decode(env.request(http.MethodPost, "/api/vacancies/"+itoa(created.ID)+"/check", nil), http.StatusOK, nil)

	// Теперь сайт лежит: прежний вывод об активности должен сохраниться.
	env.checker.set(created.URL, activity.Result{StatusCode: intPtr(503), Err: "код ответа 503 не позволяет судить об активности"})

	var checked vacancyResponse
	env.decode(env.request(http.MethodPost, "/api/vacancies/"+itoa(created.ID)+"/check", nil), http.StatusOK, &checked)

	if checked.AutoIsActive == nil || !*checked.AutoIsActive {
		t.Errorf("auto_is_active = %v, ожидалось прежнее true", checked.AutoIsActive)
	}
	if checked.LastCheckCode == nil || *checked.LastCheckCode != 503 {
		t.Errorf("last_check_code = %v, ожидалось 503", checked.LastCheckCode)
	}
	if checked.LastCheckError == "" {
		t.Error("last_check_error пуст, ожидалось описание причины")
	}
}

func TestCheckOneDoesNotOverrideManualDecision(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{"url": "https://example.com/jobs/manual"})

	// Пользователь закрыл вакансию вручную.
	var overridden vacancyResponse
	env.decode(
		env.request(http.MethodPut, "/api/vacancies/"+itoa(created.ID)+"/activity", map[string]any{"manual_is_active": false}),
		http.StatusOK, &overridden,
	)
	if overridden.IsActive {
		t.Fatal("is_active = true сразу после ручного закрытия")
	}

	// Сайт продолжает отдавать 200 — главное требование этапа: решение пользователя сильнее.
	env.checker.set(created.URL, activity.Result{Active: boolPtr(true), StatusCode: intPtr(200)})

	var checked vacancyResponse
	env.decode(env.request(http.MethodPost, "/api/vacancies/"+itoa(created.ID)+"/check", nil), http.StatusOK, &checked)

	if checked.IsActive {
		t.Error("is_active = true, авто-проверка перебила ручное решение")
	}
	if checked.ManualIsActive == nil || *checked.ManualIsActive {
		t.Errorf("manual_is_active = %v, ожидалось прежнее false", checked.ManualIsActive)
	}
	if checked.AutoIsActive == nil || !*checked.AutoIsActive {
		t.Errorf("auto_is_active = %v, ожидалось true: результат проверки сохраняется отдельно", checked.AutoIsActive)
	}
	if !checked.ActivityConflict {
		t.Error("activity_conflict = false, ожидалось true: сайт и пользователь расходятся")
	}
}

func TestCheckOneNotFound(t *testing.T) {
	env := newTestEnv(t)

	env.decodeError(env.request(http.MethodPost, "/api/vacancies/999/check", nil), http.StatusNotFound)

	if env.checker.callCount() != 0 {
		t.Error("чекер вызывался для несуществующей вакансии")
	}
}

func TestSetActivityOverrideAndReset(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{"url": "https://example.com/jobs/reset"})
	path := "/api/vacancies/" + itoa(created.ID) + "/activity"

	// Проверка сказала «снята».
	env.checker.set(created.URL, activity.Result{Active: boolPtr(false), StatusCode: intPtr(404)})
	env.decode(env.request(http.MethodPost, "/api/vacancies/"+itoa(created.ID)+"/check", nil), http.StatusOK, nil)

	// Пользователь считает вакансию живой.
	var overridden vacancyResponse
	env.decode(env.request(http.MethodPut, path, map[string]any{"manual_is_active": true}), http.StatusOK, &overridden)

	if !overridden.IsActive {
		t.Error("is_active = false, ожидалось решение пользователя true")
	}
	if !overridden.ActivityConflict {
		t.Error("activity_conflict = false, ожидалось true")
	}

	// null снимает override — вакансия снова живёт по результату проверки.
	var reset vacancyResponse
	env.decode(env.request(http.MethodPut, path, `{"manual_is_active":null}`), http.StatusOK, &reset)

	if reset.ManualIsActive != nil {
		t.Errorf("manual_is_active = %v, ожидался null", *reset.ManualIsActive)
	}
	if reset.IsActive {
		t.Error("is_active = true, ожидалось false: вернулись к результату авто-проверки")
	}
	if reset.ActivityConflict {
		t.Error("activity_conflict = true, ожидалось false: расхождения больше нет")
	}
}

func TestSetActivityValidation(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{"url": "https://example.com/jobs/validate"})
	path := "/api/vacancies/" + itoa(created.ID) + "/activity"

	t.Run("поле обязательно", func(t *testing.T) {
		payload := env.decodeError(env.request(http.MethodPut, path, `{}`), http.StatusBadRequest)
		if _, ok := payload.Fields["manual_is_active"]; !ok {
			t.Errorf("в fields нет manual_is_active: %v", payload.Fields)
		}
	})

	t.Run("неверный тип", func(t *testing.T) {
		payload := env.decodeError(env.request(http.MethodPut, path, `{"manual_is_active":"да"}`), http.StatusBadRequest)
		if payload.Code != CodeInvalidJSON {
			t.Errorf("code = %q, ожидалось %q", payload.Code, CodeInvalidJSON)
		}
	})

	t.Run("несуществующая вакансия", func(t *testing.T) {
		env.decodeError(
			env.request(http.MethodPut, "/api/vacancies/999/activity", map[string]any{"manual_is_active": true}),
			http.StatusNotFound,
		)
	})
}

func TestSetActivityBumpsUpdatedAt(t *testing.T) {
	env := newTestEnv(t)

	first := env.createVacancy(map[string]any{"url": "https://example.com/jobs/one"})
	second := env.createVacancy(map[string]any{"url": "https://example.com/jobs/two"})

	touch(t, env, first.ID, time.Now().UTC().Add(-time.Hour))
	touch(t, env, second.ID, time.Now().UTC().Add(-2*time.Hour))

	// Ручное действие пользователя должно поднять вакансию в списке.
	env.decode(
		env.request(http.MethodPut, "/api/vacancies/"+itoa(second.ID)+"/activity", map[string]any{"manual_is_active": true}),
		http.StatusOK, nil,
	)

	items := env.listVacancies(t, "")
	if len(items) == 0 || items[0].ID != second.ID {
		t.Errorf("первой отдана вакансия %v, ожидалась %d", ids(items), second.ID)
	}
}

func TestCheckDoesNotBumpUpdatedAt(t *testing.T) {
	env := newTestEnv(t)

	first := env.createVacancy(map[string]any{"url": "https://example.com/jobs/one"})
	second := env.createVacancy(map[string]any{"url": "https://example.com/jobs/two"})

	touch(t, env, first.ID, time.Now().UTC().Add(-time.Hour))
	touch(t, env, second.ID, time.Now().UTC().Add(-2*time.Hour))

	// Проверка активности — не правка карточки, порядок в таблице меняться не должен.
	env.decode(env.request(http.MethodPost, "/api/vacancies/"+itoa(second.ID)+"/check", nil), http.StatusOK, nil)

	items := env.listVacancies(t, "")
	if len(items) == 0 || items[0].ID != first.ID {
		t.Errorf("порядок изменился: %v, ожидалась первой вакансия %d", ids(items), first.ID)
	}
}

func TestCheckAllSummary(t *testing.T) {
	env := newTestEnv(t)

	alive := env.createVacancy(map[string]any{"url": "https://example.com/jobs/alive"})
	closed := env.createVacancy(map[string]any{"url": "https://example.com/jobs/closed"})
	flaky := env.createVacancy(map[string]any{"url": "https://example.com/jobs/flaky"})
	unreachable := env.createVacancy(map[string]any{"url": "https://example.com/jobs/unreachable"})
	manuallyClosed := env.createVacancy(map[string]any{"url": "https://example.com/jobs/manual"})

	env.checker.set(alive.URL, activity.Result{Active: boolPtr(true), StatusCode: intPtr(200)})
	env.checker.set(closed.URL, activity.Result{Active: boolPtr(false), StatusCode: intPtr(404)})
	env.checker.set(flaky.URL, activity.Result{StatusCode: intPtr(429), Err: "код ответа 429"})
	env.checker.set(unreachable.URL, activity.Result{Err: "сайт недоступен"})

	env.decode(
		env.request(http.MethodPut, "/api/vacancies/"+itoa(manuallyClosed.ID)+"/activity", map[string]any{"manual_is_active": false}),
		http.StatusOK, nil,
	)

	var summary service.CheckSummary
	env.decode(env.request(http.MethodPost, "/api/vacancies/check", nil), http.StatusOK, &summary)

	if summary.Checked != 4 {
		t.Errorf("checked = %d, ожидалось 4", summary.Checked)
	}
	if summary.Skipped != 1 {
		t.Errorf("skipped = %d, ожидалось 1 (вакансия закрыта вручную)", summary.Skipped)
	}
	if summary.BecameInactive != 1 {
		t.Errorf("became_inactive = %d, ожидалось 1", summary.BecameInactive)
	}
	if summary.Unknown != 1 {
		t.Errorf("unknown = %d, ожидалось 1 (сайт ответил 429)", summary.Unknown)
	}
	if summary.Failed != 1 {
		t.Errorf("failed = %d, ожидалось 1 (ответа не было)", summary.Failed)
	}

	// Закрытую вручную вакансию не опрашиваем: решение уже принято.
	if env.checker.called(manuallyClosed.URL) {
		t.Error("чекер опрашивал вакансию, закрытую вручную")
	}
}

func TestCheckAllSkipsAlreadyInactiveOnSecondRun(t *testing.T) {
	env := newTestEnv(t)

	closed := env.createVacancy(map[string]any{"url": "https://example.com/jobs/closed"})
	env.checker.set(closed.URL, activity.Result{Active: boolPtr(false), StatusCode: intPtr(404)})

	var first service.CheckSummary
	env.decode(env.request(http.MethodPost, "/api/vacancies/check", nil), http.StatusOK, &first)
	if first.BecameInactive != 1 {
		t.Fatalf("became_inactive = %d, ожидалось 1", first.BecameInactive)
	}

	// Второй прогон: вакансия уже неактивна, повторно «стала неактивной» она не может.
	// Но опросить её нужно — вдруг вакансию вернули.
	var second service.CheckSummary
	env.decode(env.request(http.MethodPost, "/api/vacancies/check", nil), http.StatusOK, &second)

	if second.BecameInactive != 0 {
		t.Errorf("became_inactive = %d при повторном прогоне, ожидалось 0", second.BecameInactive)
	}
	if second.Checked != 1 {
		t.Errorf("checked = %d, ожидалось 1: авто-снятые вакансии продолжаем опрашивать", second.Checked)
	}
}

func TestCheckAllEmptyBase(t *testing.T) {
	env := newTestEnv(t)

	var summary service.CheckSummary
	env.decode(env.request(http.MethodPost, "/api/vacancies/check", nil), http.StatusOK, &summary)

	if summary != (service.CheckSummary{}) {
		t.Errorf("сводка = %+v, ожидались нули", summary)
	}
}

func TestCheckAllRunsConcurrently(t *testing.T) {
	env := newTestEnv(t)

	const count = 8
	for i := 0; i < count; i++ {
		env.createVacancy(map[string]any{"url": "https://example.com/jobs/" + itoa(uint(i))})
	}

	// Пул из 4 воркеров: 8 проверок по 100 мс должны уложиться примерно в 200 мс,
	// последовательный обход занял бы 800 мс.
	env.checker.delay = 100 * time.Millisecond

	start := time.Now()
	var summary service.CheckSummary
	env.decode(env.request(http.MethodPost, "/api/vacancies/check", nil), http.StatusOK, &summary)
	elapsed := time.Since(start)

	if summary.Checked != count {
		t.Errorf("checked = %d, ожидалось %d", summary.Checked, count)
	}
	if elapsed > 600*time.Millisecond {
		t.Errorf("массовая проверка заняла %v — похоже, воркеры не работают параллельно", elapsed)
	}
}

func TestCheckAllUsesConfiguredChecker(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{"url": "https://example.com/jobs/probe"})

	env.decode(env.request(http.MethodPost, "/api/vacancies/check", nil), http.StatusOK, nil)

	if !env.checker.called(created.URL) {
		t.Errorf("чекер не опрашивал %s", created.URL)
	}
}

func TestCheckAllRejectsGet(t *testing.T) {
	env := newTestEnv(t)

	rec := env.request(http.MethodGet, "/api/vacancies/check", nil)
	// GET /api/vacancies/{id} не должен перехватывать /check как id.
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("статус = %d, ожидался 404 или 405, тело: %s", rec.Code, rec.Body.String())
	}
}

func TestCheckSavesStateToDatabase(t *testing.T) {
	env := newTestEnv(t)

	created := env.createVacancy(map[string]any{"url": "https://example.com/jobs/persisted"})
	env.checker.set(created.URL, activity.Result{Active: boolPtr(false), StatusCode: intPtr(410)})

	env.decode(env.request(http.MethodPost, "/api/vacancies/"+itoa(created.ID)+"/check", nil), http.StatusOK, nil)

	// Проверяем именно БД: ответ мог бы быть собран из памяти.
	var stored model.Vacancy
	if err := env.db.First(&stored, created.ID).Error; err != nil {
		t.Fatalf("чтение вакансии: %v", err)
	}
	if stored.AutoIsActive == nil || *stored.AutoIsActive {
		t.Errorf("auto_is_active в БД = %v, ожидалось false", stored.AutoIsActive)
	}
	if stored.LastCheckCode == nil || *stored.LastCheckCode != 410 {
		t.Errorf("last_check_code в БД = %v, ожидалось 410", stored.LastCheckCode)
	}
	if stored.LastCheckedAt == nil {
		t.Error("last_checked_at в БД не заполнен")
	}
}

func ids(items []vacancyResponse) []uint {
	result := make([]uint, 0, len(items))
	for _, item := range items {
		result = append(result, item.ID)
	}
	return result
}
