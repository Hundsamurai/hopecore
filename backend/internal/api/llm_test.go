package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/Hundsamurai/hopecore/backend/internal/llm"
)

func TestListProvidersEmptyWithoutKeys(t *testing.T) {
	env := newTestEnv(t)

	var resp providersResponse
	env.decode(env.request(http.MethodGet, "/api/llm/providers", nil), http.StatusOK, &resp)

	// Отсутствие ключей — не ошибка: приложение работает как на Этапе 1.
	if resp.Items == nil {
		t.Error("items = null, ожидался пустой массив")
	}
	if len(resp.Items) != 0 {
		t.Errorf("провайдеров: %d, ожидалось 0", len(resp.Items))
	}
}

func TestListProvidersShowsOnlyConfigured(t *testing.T) {
	env := newTestEnv(t)

	env.withLLM(llm.Config{Providers: []llm.ProviderConfig{
		{
			ID:     llm.ProviderGemini,
			Label:  "Gemini",
			APIKey: "test-key",
			Models: []string{"gemini-2.5-flash", "gemini-2.5-pro"},
			Price:  llm.Pricing{InputPerMillion: 0.3, OutputPerMillion: 2.5},
		},
		// Без ключа — не показывается.
		{ID: llm.ProviderClaude, Label: "Claude", Models: []string{"claude-sonnet-4-5"}},
		// С ключом, но без моделей — тоже: вызывать нечего.
		{ID: llm.ProviderDeepSeek, Label: "DeepSeek", APIKey: "test-key"},
	}})

	var resp providersResponse
	env.decode(env.request(http.MethodGet, "/api/llm/providers", nil), http.StatusOK, &resp)

	if len(resp.Items) != 1 {
		t.Fatalf("провайдеров: %d, ожидался 1: %+v", len(resp.Items), resp.Items)
	}

	got := resp.Items[0]
	if got.ID != llm.ProviderGemini || got.Label != "Gemini" {
		t.Errorf("провайдер = %+v", got)
	}
	if len(got.Models) != 2 || got.Models[0] != "gemini-2.5-flash" {
		t.Errorf("models = %v", got.Models)
	}
	// Первая модель предвыбирается в интерфейсе, поэтому она же по умолчанию.
	if got.DefaultModel != "gemini-2.5-flash" {
		t.Errorf("default_model = %q", got.DefaultModel)
	}
	if !got.PriceKnown {
		t.Error("price_known = false, хотя цена задана")
	}
}

func TestListProvidersNeverLeaksAPIKey(t *testing.T) {
	env := newTestEnv(t)

	const secret = "super-secret-key-value"
	env.withLLM(llm.Config{Providers: []llm.ProviderConfig{{
		ID:     llm.ProviderGemini,
		Label:  "Gemini",
		APIKey: secret,
		Models: []string{"gemini-2.5-flash"},
	}}})

	rec := env.request(http.MethodGet, "/api/llm/providers", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("статус = %d", rec.Code)
	}

	// Главная проверка этого эндпоинта: ключ не должен утечь ни в каком виде.
	if strings.Contains(rec.Body.String(), secret) {
		t.Fatalf("ключ попал в ответ: %s", rec.Body.String())
	}
	for _, forbidden := range []string{"api_key", "apiKey", "APIKey", "key"} {
		if strings.Contains(rec.Body.String(), forbidden) {
			t.Errorf("в ответе есть поле %q: %s", forbidden, rec.Body.String())
		}
	}
}

func TestListProvidersPriceUnknown(t *testing.T) {
	env := newTestEnv(t)

	env.withLLM(llm.Config{Providers: []llm.ProviderConfig{{
		ID:     llm.ProviderDeepSeek,
		Label:  "DeepSeek",
		APIKey: "test-key",
		Models: []string{"deepseek-chat"},
	}}})

	var resp providersResponse
	env.decode(env.request(http.MethodGet, "/api/llm/providers", nil), http.StatusOK, &resp)

	if len(resp.Items) != 1 {
		t.Fatalf("провайдеров: %d", len(resp.Items))
	}
	// Без прайса интерфейс показывает только токены, без суммы.
	if resp.Items[0].PriceKnown {
		t.Error("price_known = true, хотя цена не задана")
	}
}

func TestListProvidersRejectsPost(t *testing.T) {
	env := newTestEnv(t)

	rec := env.request(http.MethodPost, "/api/llm/providers", nil)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("статус = %d, ожидался %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
