package fetcher

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const vacancyPage = `<!doctype html>
<html lang="ru">
<head>
  <title>Go-разработчик — Пример</title>
  <meta name="description" content="Ищем Go-разработчика, вилка 300&nbsp;000 — 450&nbsp;000 ₽">
  <style>.hidden { display: none }</style>
  <script>window.analytics = {track: function(){}}</script>
</head>
<body>
  <nav>Главная Вакансии Контакты</nav>
  <header><h1>Go-разработчик</h1></header>
  <main>
    <p>Компания: ООО «Пример»</p>
    <p>Опыт: от 3 лет</p>
    <h2>Обязанности</h2>
    <ul>
      <li>Разрабатывать сервисы на Go и поддерживать существующие</li>
      <li>Проектировать схемы данных и оптимизировать запросы</li>
      <li>Участвовать в код-ревью и развивать инженерную культуру команды</li>
    </ul>
    <h2>Требования</h2>
    <ul><li>Go</li><li>PostgreSQL</li></ul>
    <p>
      Мы ищем инженера, который любит доводить задачи до продакшена и не боится
      разбираться в чужом коде. В команде шесть разработчиков, релизы раз в неделю,
      всё автоматизировано. Работаем без переработок и жёсткого контроля часов.
    </p>
    <p>Формат: удалённо. Оплата &amp; бонусы обсуждаются.</p>
  </main>
  <aside>Похожие вакансии: Python-разработчик, Java-разработчик</aside>
  <footer>© 2026 Пример. Все права защищены.</footer>
  <noscript>Включите JavaScript</noscript>
</body>
</html>`

// newTestFetcher поднимает сервер с заданным обработчиком и фетчер к нему.
func newTestFetcher(t *testing.T, handler http.HandlerFunc, maxChars int) (*Fetcher, string) {
	t.Helper()

	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return New(5*time.Second, maxChars), srv.URL
}

func htmlHandler(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}
}

func TestFetchExtractsVacancyText(t *testing.T) {
	f, url := newTestFetcher(t, htmlHandler(vacancyPage), 40000)

	page, err := f.Fetch(context.Background(), url)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if page.StatusCode != http.StatusOK {
		t.Errorf("status = %d", page.StatusCode)
	}
	if page.Title != "Go-разработчик — Пример" {
		t.Errorf("title = %q", page.Title)
	}

	// Содержимое страницы должно доехать до модели.
	for _, want := range []string{
		"Go-разработчик",
		"ООО «Пример»",
		"Опыт: от 3 лет",
		"PostgreSQL",
		"удалённо",
	} {
		if !strings.Contains(page.Text, want) {
			t.Errorf("в тексте нет %q", want)
		}
	}

	// А служебное — нет: иначе модель начнёт собирать данные чужих вакансий.
	for _, unwanted := range []string{
		"window.analytics",    // script
		"display: none",       // style
		"Главная Вакансии",    // nav
		"Похожие вакансии",    // aside
		"Все права защищены",  // footer
		"Включите JavaScript", // noscript
	} {
		if strings.Contains(page.Text, unwanted) {
			t.Errorf("в текст попало служебное содержимое %q", unwanted)
		}
	}

	if page.Chars == 0 {
		t.Error("Chars = 0")
	}
	if page.Truncated {
		t.Error("Truncated = true, хотя лимит большой")
	}
	if len(page.Warnings) != 0 {
		t.Errorf("предупреждения на нормальной странице: %v", page.Warnings)
	}
}

func TestFetchDecodesEntitiesAndMeta(t *testing.T) {
	f, url := newTestFetcher(t, htmlHandler(vacancyPage), 40000)

	page, err := f.Fetch(context.Background(), url)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// Описание из meta идёт в текст: у сайтов вакансий там часто вилка и город.
	if !strings.Contains(page.Text, "Ищем Go-разработчика") {
		t.Errorf("описание из meta не попало в текст: %q", page.Text)
	}
	// Сущности раскодированы, неразрывные пробелы схлопнуты в обычные.
	if !strings.Contains(page.Text, "300 000 — 450 000 ₽") {
		t.Errorf("сущности не раскодированы: %q", page.Text)
	}
	if !strings.Contains(page.Text, "Оплата & бонусы") {
		t.Errorf("&amp; не раскодирован: %q", page.Text)
	}
	if strings.Contains(page.Text, "&nbsp;") || strings.Contains(page.Text, "&amp;") {
		t.Errorf("в тексте остались html-сущности: %q", page.Text)
	}
}

func TestFetchKeepsJobPostingStructuredData(t *testing.T) {
	body := `<html><head><title>Вакансия</title>
	<script type="application/ld+json">{"@type":"BreadcrumbList","name":"крошки"}</script>
	<script type="application/ld+json">
	{"@context":"https://schema.org","@type":"JobPosting","title":"Senior Go Developer",
	 "datePosted":"2026-08-01","baseSalary":{"@type":"MonetaryAmount","currency":"RUB"}}
	</script>
	</head><body><p>Текст вакансии</p></body></html>`

	f, url := newTestFetcher(t, htmlHandler(body), 40000)

	page, err := f.Fetch(context.Background(), url)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// Разметка JobPosting — самые точные данные на странице, их отдаём модели.
	if !strings.Contains(page.Text, "Senior Go Developer") {
		t.Errorf("данные JobPosting не попали в текст: %q", page.Text)
	}
	if !strings.Contains(page.Text, "datePosted") {
		t.Error("в тексте нет полей JobPosting")
	}
	// А разметка хлебных крошек — шум.
	if strings.Contains(page.Text, "BreadcrumbList") || strings.Contains(page.Text, "крошки") {
		t.Errorf("в текст попала посторонняя разметка: %q", page.Text)
	}
}

func TestFetchTruncatesByLimit(t *testing.T) {
	long := "<html><body><p>" + strings.Repeat("описание вакансии ", 500) + "</p></body></html>"
	f, url := newTestFetcher(t, htmlHandler(long), 200)

	page, err := f.Fetch(context.Background(), url)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if !page.Truncated {
		t.Error("Truncated = false, хотя текст длиннее лимита")
	}
	if page.Chars > 200 {
		t.Errorf("Chars = %d, ожидалось не больше 200", page.Chars)
	}
	if !hasWarningAbout(page.Warnings, "обрезан") {
		t.Errorf("нет предупреждения об обрезке: %v", page.Warnings)
	}
}

func TestFetchWarnsOnAlmostEmptyPage(t *testing.T) {
	// Так выглядит каркас SPA: разметка есть, текста нет.
	spa := `<html><head><title>Вакансия</title></head><body><div id="root"></div>
	<script>renderApp()</script></body></html>`

	f, url := newTestFetcher(t, htmlHandler(spa), 40000)

	page, err := f.Fetch(context.Background(), url)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	// Запуск не блокируется, но пользователь должен понимать, почему пусто.
	if !hasWarningAbout(page.Warnings, "JavaScript") {
		t.Errorf("нет предупреждения о пустой странице: %v", page.Warnings)
	}
}

func TestFetchWarnsOnAntibotPage(t *testing.T) {
	// Ozon.tech отдаёт именно такую страницу.
	antibot := `<html><head><title>Antibot Challenge Page</title></head>
	<body><p>Попробуйте обновить страницу или воспользоваться другим браузером</p></body></html>`

	f, url := newTestFetcher(t, htmlHandler(antibot), 40000)

	page, err := f.Fetch(context.Background(), url)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if !hasWarningAbout(page.Warnings, "проверки браузера") {
		t.Errorf("нет предупреждения об антиботе: %v", page.Warnings)
	}
}

func TestFetchFollowsRedirectsWithCookies(t *testing.T) {
	// Реальный сценарий с ozon.tech: сайт ставит cookie и редиректит обратно.
	// Без банки cookie это бесконечный цикл перенаправлений.
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := r.Cookie("checked"); err != nil {
			http.SetCookie(w, &http.Cookie{Name: "checked", Value: "1", Path: "/"})
			http.Redirect(w, r, srv.URL+"/vacancy", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><head><title>Вакансия</title></head>
		<body><p>` + strings.Repeat("текст вакансии ", 60) + `</p></body></html>`))
	}))
	defer srv.Close()

	page, err := New(5*time.Second, 40000).Fetch(context.Background(), srv.URL+"/vacancy")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}

	if !strings.Contains(page.Text, "текст вакансии") {
		t.Errorf("страница не прочитана: %q", page.Text)
	}
	if page.FinalURL == "" {
		t.Error("FinalURL пуст")
	}
}

func TestFetchRedirectLoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/loop", http.StatusFound)
	}))
	defer srv.Close()

	_, err := New(5*time.Second, 40000).Fetch(context.Background(), srv.URL)

	assertKind(t, err, KindNetwork)
}

func TestFetchErrorKinds(t *testing.T) {
	t.Run("404", func(t *testing.T) {
		f, url := newTestFetcher(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}, 40000)

		_, err := f.Fetch(context.Background(), url)
		var fetchErr *Error
		if !errors.As(err, &fetchErr) {
			t.Fatalf("err = %v, ожидался *fetcher.Error", err)
		}
		if fetchErr.Kind != KindStatus || fetchErr.StatusCode != http.StatusNotFound {
			t.Errorf("kind = %q, status = %d", fetchErr.Kind, fetchErr.StatusCode)
		}
	})

	t.Run("403 антибот", func(t *testing.T) {
		f, url := newTestFetcher(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
		}, 40000)

		_, err := f.Fetch(context.Background(), url)
		var fetchErr *Error
		if !errors.As(err, &fetchErr) {
			t.Fatalf("err = %v", err)
		}
		if fetchErr.StatusCode != http.StatusForbidden {
			t.Errorf("status = %d", fetchErr.StatusCode)
		}
	})

	t.Run("не HTML", func(t *testing.T) {
		f, url := newTestFetcher(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF-1.4"))
		}, 40000)

		_, err := f.Fetch(context.Background(), url)
		assertKind(t, err, KindContentType)
	})

	t.Run("таймаут", func(t *testing.T) {
		release := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			<-release
		}))
		defer srv.Close()
		defer close(release)

		_, err := New(50*time.Millisecond, 40000).Fetch(context.Background(), srv.URL)
		assertKind(t, err, KindTimeout)
	})

	t.Run("недоступный хост", func(t *testing.T) {
		_, err := New(2*time.Second, 40000).Fetch(context.Background(), "http://127.0.0.1:1/vacancy")
		assertKind(t, err, KindNetwork)
	})

	t.Run("прерванный контекст", func(t *testing.T) {
		f, url := newTestFetcher(t, htmlHandler(vacancyPage), 40000)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := f.Fetch(ctx, url)
		assertKind(t, err, KindNetwork)
	})

	t.Run("битая ссылка", func(t *testing.T) {
		_, err := New(time.Second, 40000).Fetch(context.Background(), "http://%zz")
		assertKind(t, err, KindInvalidURL)
	})

	t.Run("неподдерживаемая схема", func(t *testing.T) {
		_, err := New(time.Second, 40000).Fetch(context.Background(), "ftp://example.com/vacancy")
		assertKind(t, err, KindInvalidURL)
	})
}

func TestFetchAcceptsPlainTextAndMissingContentType(t *testing.T) {
	t.Run("text/plain", func(t *testing.T) {
		f, url := newTestFetcher(t, func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			_, _ = w.Write([]byte(strings.Repeat("вакансия Go-разработчика ", 40)))
		}, 40000)

		page, err := f.Fetch(context.Background(), url)
		if err != nil {
			t.Fatalf("Fetch: %v", err)
		}
		if !strings.Contains(page.Text, "Go-разработчика") {
			t.Errorf("текст не прочитан: %q", page.Text)
		}
	})

	t.Run("без заголовка", func(t *testing.T) {
		// Отсутствие Content-Type — не повод отказываться: предполагаем HTML.
		if err := checkContentType(""); err != nil {
			t.Errorf("пустой Content-Type отклонён: %v", err)
		}
	})
}

func TestUserAgentIsASCII(t *testing.T) {
	for i := 0; i < len(UserAgent); i++ {
		if UserAgent[i] > 127 {
			t.Fatalf("User-Agent содержит не-ASCII байт в позиции %d: %q", i, UserAgent)
		}
	}
}

func TestFetchSendsUserAgent(t *testing.T) {
	var got string
	f, url := newTestFetcher(t, func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(vacancyPage))
	}, 40000)

	if _, err := f.Fetch(context.Background(), url); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got != UserAgent {
		t.Errorf("User-Agent = %q, ожидался %q", got, UserAgent)
	}
}

func TestFetchLimitsBodySize(t *testing.T) {
	// Отдаём больше, чем разрешено читать: фетчер не должен пытаться
	// проглотить страницу целиком.
	f, url := newTestFetcher(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		chunk := []byte("<p>" + strings.Repeat("a", 64*1024) + "</p>")
		for i := 0; i < (maxBodyBytes/len(chunk))+5; i++ {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	}, 0)

	page, err := f.Fetch(context.Background(), url)
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if page.Chars > maxBodyBytes {
		t.Errorf("прочитано %d символов, лимит тела — %d байт", page.Chars, maxBodyBytes)
	}
}

func assertKind(t *testing.T, err error, want ErrorKind) {
	t.Helper()

	if err == nil {
		t.Fatalf("ожидалась ошибка вида %q", want)
	}

	var fetchErr *Error
	if !errors.As(err, &fetchErr) {
		t.Fatalf("err = %v (%T), ожидался *fetcher.Error", err, err)
	}
	if fetchErr.Kind != want {
		t.Errorf("kind = %q, ожидался %q (сообщение: %s)", fetchErr.Kind, want, fetchErr.Message)
	}
	if fetchErr.Message == "" {
		t.Error("сообщение об ошибке пусто")
	}
}

func hasWarningAbout(warnings []string, fragment string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, fragment) {
			return true
		}
	}
	return false
}

// Проверка, что предупреждение о коротком тексте формулируется с числом.
func TestWarningMentionsCharCount(t *testing.T) {
	page := Page{Chars: 42}
	got := warnings(page, content{})

	if len(got) == 0 || !strings.Contains(got[0], fmt.Sprint(42)) {
		t.Errorf("предупреждение без числа символов: %v", got)
	}
}
