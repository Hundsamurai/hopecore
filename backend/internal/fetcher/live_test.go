package fetcher

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

// TestFetchLivePages проверяет фетчер на настоящих страницах вакансий.
//
// В обычном прогоне пропускается: тесты не должны зависеть от сети и от того,
// что сегодня отвечает чужой сайт. Запуск вручную:
//
//	HOPECORE_LIVE=1 go test ./internal/fetcher -run Live -v
func TestFetchLivePages(t *testing.T) {
	if os.Getenv("HOPECORE_LIVE") == "" {
		t.Skip("живая проверка выключена, включается через HOPECORE_LIVE=1")
	}

	pages := []struct {
		name string
		url  string
	}{
		{name: "rabota.sber.ru — одна вакансия", url: "https://rabota.sber.ru/search/middle-golang-razrabochik-4554633/"},
		{name: "sveak.com — список вакансий", url: "https://sveak.com/vacancy"},
		{name: "ozon.tech — антибот", url: "https://ozon.tech/vacancies/135943148/?__rr=1&abt_att=1"},
	}

	f := New(15*time.Second, 40000)

	for _, page := range pages {
		t.Run(page.name, func(t *testing.T) {
			got, err := f.Fetch(context.Background(), page.url)
			if err != nil {
				var fetchErr *Error
				if errors.As(err, &fetchErr) {
					t.Logf("не скачалось: вид=%s код=%d — %s", fetchErr.Kind, fetchErr.StatusCode, fetchErr.Message)
					return
				}
				t.Logf("не скачалось: %v", err)
				return
			}

			t.Logf("код=%d символов=%d обрезано=%v", got.StatusCode, got.Chars, got.Truncated)
			t.Logf("заголовок: %s", got.Title)
			for _, warning := range got.Warnings {
				t.Logf("предупреждение: %s", warning)
			}

			preview := got.Text
			if len([]rune(preview)) > 400 {
				preview = string([]rune(preview)[:400])
			}
			t.Logf("начало текста:\n%s", strings.TrimSpace(preview))
		})
	}
}

// по этому видно, чего от модели можно ожидать, а чего нет.
//
//	HOPECORE_LIVE=1 go test ./internal/fetcher -run LiveExtractable -v
func TestLiveExtractableSignals(t *testing.T) {
	if os.Getenv("HOPECORE_LIVE") == "" {
		t.Skip("живая проверка выключена, включается через HOPECORE_LIVE=1")
	}

	page, err := New(15*time.Second, 40000).
		Fetch(context.Background(), "https://rabota.sber.ru/search/middle-golang-razrabochik-4554633/")
	if err != nil {
		t.Skipf("страница недоступна: %v", err)
	}

	signals := map[string][]string{
		"должность":     {"Golang", "разрабочик", "разработчик"},
		"компания":      {"Сбербанк", "Сбер"},
		"город":         {"Москва"},
		"грейд":         {"Middle", "миддл", "опыт"},
		"вилка":         {"₽", "руб", "зарплат", "от 1", "до 1"},
		"формат работы": {"удалён", "офис", "гибрид"},
		"дата":          {"августа", "2026"},
		"технологии":    {"Go", "PostgreSQL", "Kubernetes", "SQL"},
	}

	lower := strings.ToLower(page.Text)
	for field, markers := range signals {
		var found []string
		for _, marker := range markers {
			if strings.Contains(lower, strings.ToLower(marker)) {
				found = append(found, marker)
			}
		}
		if len(found) == 0 {
			t.Logf("%-14s НЕТ признаков в тексте", field)
			continue
		}
		t.Logf("%-14s найдено: %s", field, strings.Join(found, ", "))
	}

	t.Logf("всего символов: %d", page.Chars)
}
