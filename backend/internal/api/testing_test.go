package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/Hundsamurai/hopecore/backend/internal/activity"
	"github.com/Hundsamurai/hopecore/backend/internal/llm"
	"github.com/Hundsamurai/hopecore/backend/internal/service"
	"github.com/Hundsamurai/hopecore/backend/internal/store"
)

// stubChecker подменяет реальную сеть предсказуемыми ответами.
// Ключ — URL вакансии, значение — что «ответил» сайт.
type stubChecker struct {
	mu       sync.Mutex
	results  map[string]activity.Result
	fallback activity.Result
	calls    []string
	delay    time.Duration
}

func newStubChecker() *stubChecker {
	return &stubChecker{
		results: map[string]activity.Result{},
		// По умолчанию считаем, что сайт отдал 200.
		fallback: activity.Result{Active: boolPtr(true), StatusCode: intPtr(200)},
	}
}

func (c *stubChecker) Check(ctx context.Context, rawURL string) activity.Result {
	if c.delay > 0 {
		select {
		case <-ctx.Done():
			return activity.Result{Err: "проверка прервана"}
		case <-time.After(c.delay):
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.calls = append(c.calls, rawURL)
	if res, ok := c.results[rawURL]; ok {
		return res
	}
	return c.fallback
}

// set задаёт ответ для конкретной ссылки.
func (c *stubChecker) set(rawURL string, res activity.Result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.results[rawURL] = res
}

// callCount сообщает, сколько раз чекер вызывали.
func (c *stubChecker) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

// called сообщает, опрашивалась ли конкретная ссылка.
func (c *stubChecker) called(rawURL string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, call := range c.calls {
		if call == rawURL {
			return true
		}
	}
	return false
}

// testEnv — роутер поверх чистой in-memory БД с подставным чекером.
type testEnv struct {
	t        *testing.T
	handler  http.Handler
	db       *gorm.DB
	checker  *stubChecker
	log      *slog.Logger
	activity *service.ActivityService
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	db, err := store.OpenMemory(log)
	if err != nil {
		t.Fatalf("OpenMemory: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(db); err != nil {
			t.Errorf("Close: %v", err)
		}
	})

	if err := store.Migrate(db); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	checker := newStubChecker()
	activityService := service.NewActivityService(db, checker, 4, log)

	env := &testEnv{
		t:       t,
		db:      db,
		checker: checker,
	}
	// По умолчанию провайдеров нет: приложение должно вести себя как на Этапе 1.
	env.rebuild(log, activityService, llm.Config{})
	env.log = log
	env.activity = activityService
	return env
}

// rebuild пересобирает роутер с новой конфигурацией моделей.
func (e *testEnv) rebuild(log *slog.Logger, activity *service.ActivityService, llmConfig llm.Config) {
	e.handler = NewServer(Deps{
		Log:      log,
		DB:       e.db,
		Activity: activity,
		LLM:      llmConfig,
	}).Routes()
}

// withLLM подключает конфигурацию провайдеров к тому же окружению.
func (e *testEnv) withLLM(cfg llm.Config) {
	e.rebuild(e.log, e.activity, cfg)
}

func intPtr(v int) *int { return &v }

// request выполняет запрос к API. body может быть nil, строкой или структурой.
func (e *testEnv) request(method, path string, body any) *httptest.ResponseRecorder {
	e.t.Helper()

	var reader io.Reader
	switch v := body.(type) {
	case nil:
		reader = nil
	case string:
		reader = bytes.NewBufferString(v)
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			e.t.Fatalf("сериализация тела запроса: %v", err)
		}
		reader = bytes.NewReader(raw)
	}

	req := httptest.NewRequest(method, path, reader)
	if reader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

// decode разбирает успешный ответ, проверяя код статуса.
func (e *testEnv) decode(rec *httptest.ResponseRecorder, wantStatus int, dest any) {
	e.t.Helper()

	if rec.Code != wantStatus {
		e.t.Fatalf("статус = %d, ожидался %d, тело: %s", rec.Code, wantStatus, rec.Body.String())
	}
	if dest == nil {
		return
	}
	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		e.t.Fatalf("разбор ответа: %v, тело: %s", err, rec.Body.String())
	}
}

// decodeError разбирает ответ с ошибкой в едином формате.
func (e *testEnv) decodeError(rec *httptest.ResponseRecorder, wantStatus int) errorPayload {
	e.t.Helper()

	if rec.Code != wantStatus {
		e.t.Fatalf("статус = %d, ожидался %d, тело: %s", rec.Code, wantStatus, rec.Body.String())
	}

	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		e.t.Fatalf("разбор ошибки: %v, тело: %s", err, rec.Body.String())
	}
	if body.Error.Code == "" {
		e.t.Fatalf("в ответе нет error.code, тело: %s", rec.Body.String())
	}
	return body.Error
}

// createVacancy — хелпер: создаёт вакансию через API и возвращает её представление.
func (e *testEnv) createVacancy(body any) vacancyResponse {
	e.t.Helper()

	rec := e.request(http.MethodPost, "/api/vacancies", body)

	var created vacancyResponse
	e.decode(rec, http.StatusCreated, &created)
	return created
}
