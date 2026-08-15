package gemini_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/Hundsamurai/hopecore/backend/internal/fetcher"
	"github.com/Hundsamurai/hopecore/backend/internal/llm"
	"github.com/Hundsamurai/hopecore/backend/internal/llm/gemini"
)

// TestLiveExtraction проверяет всю связку на настоящей странице: скачивание,
// очистка, запрос к Gemini, разбор и валидация ответа.
//
// В обычном прогоне пропускается — нужен ключ и сеть. Запуск:
//
//	HOPECORE_LIVE=1 GEMINI_API_KEY=... go test ./internal/llm/gemini -run Live -v
func TestLiveExtraction(t *testing.T) {
	if os.Getenv("HOPECORE_LIVE") == "" {
		t.Skip("живая проверка выключена, включается через HOPECORE_LIVE=1")
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		t.Skip("нет GEMINI_API_KEY")
	}

	pages := []struct {
		name string
		url  string
		// Чего ждём от страницы: у Сбера вилки нет, и модель обязана вернуть null.
		expectSalary bool
	}{
		{
			name: "rabota.sber.ru",
			url:  "https://rabota.sber.ru/search/middle-golang-razrabochik-4554633/",
		},
	}

	provider := gemini.New(apiKey, 60*time.Second)

	for _, page := range pages {
		t.Run(page.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()

			fetched, err := fetcher.New(15*time.Second, 40000).Fetch(ctx, page.url)
			if err != nil {
				t.Skipf("страница недоступна: %v", err)
			}
			t.Logf("страница: %d символов, предупреждений: %d", fetched.Chars, len(fetched.Warnings))

			start := time.Now()
			resp, err := provider.Complete(ctx, llm.Request{
				Model:  "gemini-2.5-flash",
				System: llm.SystemPrompt,
				User:   llm.BuildUserPrompt(page.url, fetched.Text),
				Schema: llm.ExtractionSchema(),
			})
			if err != nil {
				t.Fatalf("Complete: %v", err)
			}

			t.Logf("токены: вход=%d выход=%d, время=%s",
				resp.InputTokens, resp.OutputTokens, time.Since(start).Round(time.Millisecond))
			t.Logf("сырой ответ: %s", resp.JSON)

			result, err := llm.Parse(resp.JSON)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			pretty, _ := json.MarshalIndent(result.Values, "", "  ")
			t.Logf("разобранные значения:\n%s", pretty)
			for _, note := range result.Notes {
				t.Logf("замечание по %s: %s", note.Field, note.Note)
			}

			// Минимальное ожидание: должность и компания на странице точно есть.
			if !result.Filled["title"] {
				t.Error("должность не извлечена, хотя на странице она есть")
			}
			if !result.Filled["company"] {
				t.Error("компания не извлечена, хотя на странице она есть")
			}

			// Главная проверка правила «не выдумывай»: вилки на странице нет.
			if !page.expectSalary && result.Values.SalaryFrom != nil {
				t.Errorf("модель выдумала вилку: salary_from = %v, хотя на странице цифр нет",
					*result.Values.SalaryFrom)
			}
		})
	}
}
