package activity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		status int
		want   *bool
	}{
		{status: 200, want: ptr(true)},
		{status: 201, want: ptr(true)},
		{status: 299, want: ptr(true)},
		{status: 404, want: ptr(false)},
		{status: 410, want: ptr(false)},
		// Ниже — случаи, когда о вакансии ничего не известно.
		{status: 301, want: nil},
		{status: 401, want: nil},
		{status: 403, want: nil},
		{status: 429, want: nil},
		{status: 500, want: nil},
		{status: 503, want: nil},
	}

	for _, tc := range cases {
		got := Classify(tc.status)
		switch {
		case tc.want == nil && got != nil:
			t.Errorf("Classify(%d) = %v, ожидался nil (неизвестно)", tc.status, *got)
		case tc.want != nil && got == nil:
			t.Errorf("Classify(%d) = nil, ожидалось %v", tc.status, *tc.want)
		case tc.want != nil && got != nil && *got != *tc.want:
			t.Errorf("Classify(%d) = %v, ожидалось %v", tc.status, *got, *tc.want)
		}
	}
}

func TestHTTPCheckerStatusCodes(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		wantActive *bool
		wantErr    bool
	}{
		{name: "200 — вакансия активна", status: 200, wantActive: ptr(true)},
		{name: "404 — вакансия снята", status: 404, wantActive: ptr(false)},
		{name: "410 — вакансия снята", status: 410, wantActive: ptr(false)},
		{name: "500 — неизвестно", status: 500, wantActive: nil, wantErr: true},
		{name: "429 — неизвестно", status: 429, wantActive: nil, wantErr: true},
		{name: "403 — неизвестно", status: 403, wantActive: nil, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.Header.Get("User-Agent"); got != UserAgent {
					t.Errorf("User-Agent = %q, ожидался %q", got, UserAgent)
				}
				if r.Method != http.MethodGet {
					t.Errorf("метод = %s, ожидался GET", r.Method)
				}
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			res := NewHTTPChecker(5*time.Second).Check(context.Background(), srv.URL)

			if res.StatusCode == nil || *res.StatusCode != tc.status {
				t.Fatalf("status_code = %v, ожидался %d", res.StatusCode, tc.status)
			}
			assertActive(t, res.Active, tc.wantActive)

			if tc.wantErr && res.Err == "" {
				t.Error("ожидалось пояснение в Err")
			}
			if !tc.wantErr && res.Err != "" {
				t.Errorf("Err = %q, ожидалась пустая строка", res.Err)
			}
		})
	}
}

func TestHTTPCheckerFollowsRedirects(t *testing.T) {
	var target *httptest.Server
	target = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/final":
			w.WriteHeader(http.StatusOK)
		default:
			http.Redirect(w, r, target.URL+"/final", http.StatusFound)
		}
	}))
	defer target.Close()

	res := NewHTTPChecker(5*time.Second).Check(context.Background(), target.URL+"/start")

	if res.StatusCode == nil || *res.StatusCode != http.StatusOK {
		t.Fatalf("status_code = %v, ожидался 200 после редиректа", res.StatusCode)
	}
	assertActive(t, res.Active, ptr(true))
}

func TestHTTPCheckerRedirectOnClosedVacancy(t *testing.T) {
	// Частый случай: снятая вакансия редиректит на поиск, который отдаёт 200.
	// Проверка честно вернёт «активна» — это и есть тот самый ложно-положительный
	// результат, который закрывается ручным override (риски в п. 10 дизайна).
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/search" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Redirect(w, r, srv.URL+"/search", http.StatusMovedPermanently)
	}))
	defer srv.Close()

	res := NewHTTPChecker(5*time.Second).Check(context.Background(), srv.URL+"/vacancy/1")

	assertActive(t, res.Active, ptr(true))
}

func TestHTTPCheckerRedirectLoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	defer srv.Close()

	res := NewHTTPChecker(5*time.Second).Check(context.Background(), srv.URL)

	if !res.Unknown() {
		t.Errorf("Active = %v, ожидался nil: цикл редиректов ничего не говорит о вакансии", *res.Active)
	}
	if res.Err == "" {
		t.Error("ожидалось сообщение об ошибке")
	}
}

func TestHTTPCheckerTimeout(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer close(release)

	res := NewHTTPChecker(50*time.Millisecond).Check(context.Background(), srv.URL)

	if !res.Unknown() {
		t.Fatalf("Active = %v, ожидался nil при таймауте", *res.Active)
	}
	if res.StatusCode != nil {
		t.Errorf("status_code = %v, ожидался nil: ответа не было", *res.StatusCode)
	}
	if !strings.Contains(res.Err, "таймаут") {
		t.Errorf("Err = %q, ожидалось упоминание таймаута", res.Err)
	}
}

func TestHTTPCheckerUnreachableHost(t *testing.T) {
	// Порт 1 на localhost гарантированно закрыт — соединение отвергается.
	res := NewHTTPChecker(2*time.Second).Check(context.Background(), "http://127.0.0.1:1/vacancy")

	if !res.Unknown() {
		t.Fatalf("Active = %v, ожидался nil для недоступного хоста", *res.Active)
	}
	if res.Err == "" {
		t.Error("ожидалось сообщение об ошибке")
	}
}

func TestHTTPCheckerCancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res := NewHTTPChecker(5*time.Second).Check(ctx, srv.URL)

	if !res.Unknown() {
		t.Fatalf("Active = %v, ожидался nil для отменённого контекста", *res.Active)
	}
	if !strings.Contains(res.Err, "прерван") {
		t.Errorf("Err = %q, ожидалось упоминание прерывания", res.Err)
	}
}

func TestHTTPCheckerInvalidURL(t *testing.T) {
	res := NewHTTPChecker(time.Second).Check(context.Background(), "http://%zz")

	if !res.Unknown() {
		t.Fatalf("Active = %v, ожидался nil", *res.Active)
	}
	if res.Err == "" {
		t.Error("ожидалось сообщение об ошибке")
	}
}

func TestApplyResult(t *testing.T) {
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)

	t.Run("определённый результат обновляет auto", func(t *testing.T) {
		v := model.Vacancy{AutoIsActive: ptr(true)}
		code := 404

		ApplyResult(&v, Result{Active: ptr(false), StatusCode: &code}, now)

		if v.AutoIsActive == nil || *v.AutoIsActive {
			t.Errorf("auto_is_active = %v, ожидалось false", v.AutoIsActive)
		}
		if v.LastCheckedAt == nil || !v.LastCheckedAt.Equal(now) {
			t.Errorf("last_checked_at = %v, ожидалось %v", v.LastCheckedAt, now)
		}
		if v.LastCheckCode == nil || *v.LastCheckCode != 404 {
			t.Errorf("last_check_code = %v, ожидалось 404", v.LastCheckCode)
		}
		if v.LastCheckError != "" {
			t.Errorf("last_check_error = %q, ожидалась пустая строка", v.LastCheckError)
		}
	})

	t.Run("неизвестный результат не затирает auto", func(t *testing.T) {
		v := model.Vacancy{AutoIsActive: ptr(true)}
		code := 503

		ApplyResult(&v, Result{StatusCode: &code, Err: "код ответа 503"}, now)

		if v.AutoIsActive == nil || !*v.AutoIsActive {
			t.Errorf("auto_is_active = %v, ожидалось прежнее true", v.AutoIsActive)
		}
		if v.LastCheckError == "" {
			t.Error("last_check_error пуст, ожидалось описание причины")
		}
		if v.LastCheckedAt == nil {
			t.Error("last_checked_at не заполнен: попытка проверки всё равно была")
		}
	})

	t.Run("ручной override не трогается", func(t *testing.T) {
		v := model.Vacancy{ManualIsActive: ptr(false)}

		ApplyResult(&v, Result{Active: ptr(true), StatusCode: ptr(200)}, now)

		if v.ManualIsActive == nil || *v.ManualIsActive {
			t.Errorf("manual_is_active = %v, ожидалось прежнее false", v.ManualIsActive)
		}
		// Эффективное значение остаётся за пользователем, но расхождение видно.
		active, conflict := Resolve(v.AutoIsActive, v.ManualIsActive)
		if active {
			t.Error("effective = true, ожидалось решение пользователя false")
		}
		if !conflict {
			t.Error("conflict = false, ожидалось true")
		}
	})

	t.Run("ошибка предыдущей проверки сбрасывается", func(t *testing.T) {
		v := model.Vacancy{LastCheckError: "таймаут запроса"}

		ApplyResult(&v, Result{Active: ptr(true), StatusCode: ptr(200)}, now)

		if v.LastCheckError != "" {
			t.Errorf("last_check_error = %q, ожидалась пустая строка после успешной проверки", v.LastCheckError)
		}
	})
}

func assertActive(t *testing.T, got, want *bool) {
	t.Helper()

	switch {
	case want == nil && got != nil:
		t.Errorf("Active = %v, ожидался nil", *got)
	case want != nil && got == nil:
		t.Errorf("Active = nil, ожидалось %v", *want)
	case want != nil && got != nil && *got != *want:
		t.Errorf("Active = %v, ожидалось %v", *got, *want)
	}
}

func ptr[T any](v T) *T { return &v }

func TestUserAgentIsASCII(t *testing.T) {
	// Значения HTTP-заголовков за пределами ASCII не предусмотрены стандартом.
	// Кириллица в User-Agent уже ломала сторонние клиенты при отладке.
	for i := 0; i < len(UserAgent); i++ {
		if UserAgent[i] > 127 {
			t.Fatalf("User-Agent содержит не-ASCII байт в позиции %d: %q", i, UserAgent)
		}
	}
	if UserAgent == "" {
		t.Fatal("User-Agent пуст: часть сайтов такие запросы отвергает")
	}
}
