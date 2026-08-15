package gemini

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Hundsamurai/hopecore/backend/internal/llm"
)

const testKey = "test-api-key-value"

// capturedRequest — то, что адаптер отправил провайдеру.
type capturedRequest struct {
	path   string
	apiKey string
	body   map[string]any
}

// newStub поднимает подставной Gemini и возвращает адаптер к нему.
func newStub(t *testing.T, handler func(w http.ResponseWriter, captured capturedRequest)) (*Provider, *capturedRequest) {
	t.Helper()

	captured := &capturedRequest{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.path = r.URL.Path
		captured.apiKey = r.Header.Get("x-goog-api-key")

		if err := json.NewDecoder(r.Body).Decode(&captured.body); err != nil {
			t.Errorf("тело запроса не разобрано: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		handler(w, *captured)
	}))
	t.Cleanup(srv.Close)

	provider := New(testKey, 5*time.Second)
	provider.BaseURL = srv.URL

	return provider, captured
}

func okResponse(answer string, promptTokens, answerTokens, thoughtTokens int) string {
	body := map[string]any{
		"candidates": []any{map[string]any{
			"content":      map[string]any{"parts": []any{map[string]any{"text": answer}}},
			"finishReason": "STOP",
		}},
		"usageMetadata": map[string]any{
			"promptTokenCount":     promptTokens,
			"candidatesTokenCount": answerTokens,
			"thoughtsTokenCount":   thoughtTokens,
		},
	}
	raw, _ := json.Marshal(body)
	return string(raw)
}

func testRequest() llm.Request {
	return llm.Request{
		Model:  "gemini-2.5-flash",
		System: llm.SystemPrompt,
		User:   "Текст страницы: Ищем Go-разработчика",
		Schema: llm.ExtractionSchema(),
	}
}

func TestCompleteSendsSchemaAndKey(t *testing.T) {
	provider, captured := newStub(t, func(w http.ResponseWriter, _ capturedRequest) {
		_, _ = w.Write([]byte(okResponse(`{"title":"Go-разработчик"}`, 100, 20, 0)))
	})

	resp, err := provider.Complete(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if string(resp.JSON) != `{"title":"Go-разработчик"}` {
		t.Errorf("JSON = %s", resp.JSON)
	}

	// Модель попадает в путь, а ключ — только в заголовок: в строке запроса
	// он оказался бы в логах прокси.
	if !strings.HasSuffix(captured.path, "/models/gemini-2.5-flash:generateContent") {
		t.Errorf("путь = %q", captured.path)
	}
	if captured.apiKey != testKey {
		t.Errorf("ключ в заголовке = %q", captured.apiKey)
	}
	if strings.Contains(captured.path, testKey) {
		t.Error("ключ попал в путь запроса")
	}

	config, ok := captured.body["generationConfig"].(map[string]any)
	if !ok {
		t.Fatalf("нет generationConfig: %v", captured.body)
	}
	if config["responseMimeType"] != "application/json" {
		t.Errorf("responseMimeType = %v", config["responseMimeType"])
	}
	if config["temperature"] != float64(0) {
		t.Errorf("temperature = %v, ожидался 0: извлечение не творческая задача", config["temperature"])
	}

	schema, ok := config["responseSchema"].(map[string]any)
	if !ok {
		t.Fatalf("нет responseSchema: %v", config)
	}
	if schema["type"] != "OBJECT" {
		t.Errorf("type схемы = %v, ожидался OBJECT заглавными", schema["type"])
	}

	properties := schema["properties"].(map[string]any)
	if len(properties) != len(llm.ExtractionSchema().Fields) {
		t.Errorf("свойств в схеме: %d, ожидалось %d", len(properties), len(llm.ExtractionSchema().Fields))
	}

	// Системный промпт уходит отдельным полем, а не подмешивается в текст.
	instruction, ok := captured.body["systemInstruction"].(map[string]any)
	if !ok {
		t.Fatalf("нет systemInstruction: %v", captured.body)
	}
	if !strings.Contains(firstPartText(t, instruction), "Не выдумывай") {
		t.Error("системный промпт не доехал")
	}
}

func TestSchemaTranslationDetails(t *testing.T) {
	schema := toGeminiSchema(llm.ExtractionSchema())
	properties := schema["properties"].(map[string]any)

	t.Run("типы заглавными", func(t *testing.T) {
		cases := map[string]string{
			"title":        "STRING",
			"tech_tags":    "ARRAY",
			"salary_from":  "NUMBER",
			"salary_gross": "BOOLEAN",
		}
		for field, want := range cases {
			got := properties[field].(map[string]any)["type"]
			if got != want {
				t.Errorf("%s: type = %v, ожидался %s", field, got, want)
			}
		}
	})

	t.Run("nullable вместо списка типов", func(t *testing.T) {
		date := properties["opened_date"].(map[string]any)
		if date["nullable"] != true {
			t.Errorf("opened_date: nullable = %v", date["nullable"])
		}
		if _, isList := date["type"].([]string); isList {
			t.Error("тип задан списком, а Gemini ждёт nullable")
		}
	})

	t.Run("пустая строка не попадает в enum", func(t *testing.T) {
		// Живой API отвечает на пустое значение «enum[0]: cannot be empty».
		for _, field := range []string{"grade", "work_format"} {
			values, ok := properties[field].(map[string]any)["enum"].([]string)
			if !ok {
				t.Fatalf("%s: нет enum", field)
			}
			for _, value := range values {
				if value == "" {
					t.Errorf("%s: в enum попала пустая строка", field)
				}
			}
			// Вместо неё «не нашёл» выражается через null.
			if properties[field].(map[string]any)["nullable"] != true {
				t.Errorf("%s: nullable не выставлен, модели некуда деть «не нашёл»", field)
			}
		}
	})

	t.Run("additionalProperties не отправляется", func(t *testing.T) {
		// Gemini этого поля не поддерживает.
		if _, present := schema["additionalProperties"]; present {
			t.Error("additionalProperties присутствует в схеме")
		}
	})

	t.Run("все поля обязательны", func(t *testing.T) {
		required := schema["required"].([]string)
		if len(required) != len(llm.ExtractionSchema().Fields) {
			t.Errorf("required = %v", required)
		}
	})
}

func TestThinkingDisabledByDefault(t *testing.T) {
	provider, captured := newStub(t, func(w http.ResponseWriter, _ capturedRequest) {
		_, _ = w.Write([]byte(okResponse(`{}`, 10, 5, 0)))
	})

	if _, err := provider.Complete(context.Background(), testRequest()); err != nil {
		t.Fatalf("Complete: %v", err)
	}

	config := captured.body["generationConfig"].(map[string]any)
	thinkingConfig, ok := config["thinkingConfig"].(map[string]any)
	if !ok {
		t.Fatalf("нет thinkingConfig: размышления стоят втрое дороже ответа")
	}
	if thinkingConfig["thinkingBudget"] != float64(0) {
		t.Errorf("thinkingBudget = %v, ожидался 0", thinkingConfig["thinkingBudget"])
	}
}

func TestRetryWithoutThinkingConfig(t *testing.T) {
	var attempts int
	var lastHadThinking bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++

		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		config := body["generationConfig"].(map[string]any)
		_, lastHadThinking = config["thinkingConfig"]

		w.Header().Set("Content-Type", "application/json")

		// Первый запрос отклоняем так, как это делают модели,
		// не умеющие отключать размышления.
		if lastHadThinking {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":400,"message":"Thinking config is not supported for this model","status":"INVALID_ARGUMENT"}}`))
			return
		}
		_, _ = w.Write([]byte(okResponse(`{"title":"Go-разработчик"}`, 100, 20, 40)))
	}))
	defer srv.Close()

	provider := New(testKey, 5*time.Second)
	provider.BaseURL = srv.URL

	resp, err := provider.Complete(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if attempts != 2 {
		t.Errorf("попыток: %d, ожидалось 2", attempts)
	}
	if lastHadThinking {
		t.Error("повторная попытка снова содержала thinkingConfig")
	}
	if string(resp.JSON) != `{"title":"Go-разработчик"}` {
		t.Errorf("JSON = %s", resp.JSON)
	}
}

func TestTokensIncludeThoughts(t *testing.T) {
	provider, _ := newStub(t, func(w http.ResponseWriter, _ capturedRequest) {
		_, _ = w.Write([]byte(okResponse(`{}`, 64, 52, 133)))
	})

	resp, err := provider.Complete(context.Background(), testRequest())
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if resp.InputTokens != 64 {
		t.Errorf("InputTokens = %d, ожидалось 64", resp.InputTokens)
	}
	// Токены размышлений тарифицируются как выходные. Если их не считать,
	// оценка стоимости врёт в разы: 52 против 185.
	if resp.OutputTokens != 185 {
		t.Errorf("OutputTokens = %d, ожидалось 185 (52 ответа + 133 размышлений)", resp.OutputTokens)
	}
}

func TestCompleteErrors(t *testing.T) {
	cases := []struct {
		name       string
		status     int
		body       string
		wantInText string
	}{
		{
			name:       "превышена квота",
			status:     http.StatusTooManyRequests,
			body:       `{"error":{"code":429,"message":"You exceeded your current quota","status":"RESOURCE_EXHAUSTED"}}`,
			wantInText: "квота",
		},
		{
			name:       "модель недоступна",
			status:     http.StatusNotFound,
			body:       `{"error":{"code":404,"message":"This model is no longer available to new users","status":"NOT_FOUND"}}`,
			wantInText: "недоступна",
		},
		{
			name:       "ключ отклонён",
			status:     http.StatusForbidden,
			body:       `{"error":{"code":403,"message":"API key not valid","status":"PERMISSION_DENIED"}}`,
			wantInText: "ключ",
		},
		{
			name:       "внутренняя ошибка",
			status:     http.StatusInternalServerError,
			body:       `{"error":{"code":500,"message":"Internal error"}}`,
			wantInText: "500",
		},
		{
			name:       "ответ обрезан",
			status:     http.StatusOK,
			body:       `{"candidates":[{"content":{"parts":[{"text":"{\"title\":"}]},"finishReason":"MAX_TOKENS"}],"usageMetadata":{"promptTokenCount":9000}}`,
			wantInText: "обрезан",
		},
		{
			name:       "запрос заблокирован",
			status:     http.StatusOK,
			body:       `{"promptFeedback":{"blockReason":"SAFETY"}}`,
			wantInText: "отклонил запрос",
		},
		{
			name:       "нет кандидатов",
			status:     http.StatusOK,
			body:       `{"candidates":[]}`,
			wantInText: "не вернул ответ",
		},
		{
			name:       "пустой текст",
			status:     http.StatusOK,
			body:       `{"candidates":[{"content":{"parts":[{"text":"   "}]},"finishReason":"STOP"}]}`,
			wantInText: "пустой",
		},
		{
			name:       "мусор вместо json",
			status:     http.StatusOK,
			body:       `не json`,
			wantInText: "не разобран",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			provider, _ := newStub(t, func(w http.ResponseWriter, _ capturedRequest) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})

			_, err := provider.Complete(context.Background(), testRequest())
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			if !strings.Contains(err.Error(), tc.wantInText) {
				t.Errorf("ошибка = %q, ожидалось упоминание %q", err.Error(), tc.wantInText)
			}
			// Ключ не должен просочиться в текст ошибки: она попадёт в журнал.
			if strings.Contains(err.Error(), testKey) {
				t.Error("ключ попал в сообщение об ошибке")
			}
		})
	}
}

func TestCompleteReportsTokensEvenOnFailure(t *testing.T) {
	// Обрезанный ответ уже стоил токенов, и это должно попасть в журнал.
	provider, _ := newStub(t, func(w http.ResponseWriter, _ capturedRequest) {
		_, _ = w.Write([]byte(`{"candidates":[{"finishReason":"MAX_TOKENS"}],"usageMetadata":{"promptTokenCount":9000,"candidatesTokenCount":100}}`))
	})

	resp, err := provider.Complete(context.Background(), testRequest())
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if resp.InputTokens != 9000 {
		t.Errorf("InputTokens = %d, ожидалось 9000: потраченные токены нужно записать в журнал", resp.InputTokens)
	}
}

func TestCompleteUnreachable(t *testing.T) {
	provider := New(testKey, 2*time.Second)
	provider.BaseURL = "http://127.0.0.1:1"

	_, err := provider.Complete(context.Background(), testRequest())
	if err == nil {
		t.Fatal("ожидалась ошибка")
	}
	if !strings.Contains(err.Error(), "недоступен") {
		t.Errorf("ошибка = %q", err)
	}
}

func TestCompleteRespectsContext(t *testing.T) {
	provider, _ := newStub(t, func(w http.ResponseWriter, _ capturedRequest) {
		_, _ = w.Write([]byte(okResponse(`{}`, 1, 1, 0)))
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := provider.Complete(ctx, testRequest()); err == nil {
		t.Fatal("ожидалась ошибка при отменённом контексте")
	}
}

func TestProviderID(t *testing.T) {
	if got := New(testKey, time.Second).ID(); got != llm.ProviderGemini {
		t.Errorf("ID = %q, ожидался %q", got, llm.ProviderGemini)
	}
}

func firstPartText(t *testing.T, instruction map[string]any) string {
	t.Helper()

	parts, ok := instruction["parts"].([]any)
	if !ok || len(parts) == 0 {
		t.Fatalf("нет parts: %v", instruction)
	}
	text, _ := parts[0].(map[string]any)["text"].(string)
	return text
}
