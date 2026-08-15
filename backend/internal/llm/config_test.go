package llm

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// clearKeys убирает ключи из окружения теста.
//
// Без этого тест зависит от того, что лежит в окружении запускающего:
// однажды экспортированный для живой проверки GEMINI_API_KEY уронил прогон.
func clearKeys(t *testing.T) {
	t.Helper()

	for _, key := range []string{"GEMINI_API_KEY", "ANTHROPIC_API_KEY", "DEEPSEEK_API_KEY"} {
		t.Setenv(key, "")
	}
}

func TestLoadConfigWithoutKeys(t *testing.T) {
	clearKeys(t)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	// Отсутствие ключей — рабочее состояние, а не ошибка.
	if cfg.Enabled() {
		t.Errorf("Enabled() = true без ключей: %+v", cfg.Available())
	}
	// В конфигурацию попадают только провайдеры с готовым адаптером:
	// ключ без адаптера дал бы в интерфейсе выбор, который не работает.
	if len(cfg.Providers) != len(ImplementedProviders()) {
		t.Errorf("провайдеров в конфигурации: %d, реализовано: %d",
			len(cfg.Providers), len(ImplementedProviders()))
	}
	if cfg.Timeout != defaultTimeout || cfg.FetchTimeout != defaultFetchTimeout {
		t.Errorf("таймауты по умолчанию не подставились: %v / %v", cfg.Timeout, cfg.FetchTimeout)
	}
	if cfg.MaxPageChars != defaultMaxPageChars {
		t.Errorf("MaxPageChars = %d, ожидалось %d", cfg.MaxPageChars, defaultMaxPageChars)
	}
}

func TestLoadConfigWithGeminiKey(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if !cfg.Enabled() {
		t.Fatal("Enabled() = false, хотя ключ задан")
	}

	available := cfg.Available()
	if len(available) != 1 || available[0].ID != ProviderGemini {
		t.Fatalf("доступные провайдеры: %+v", available)
	}
	// Модели по умолчанию должны быть, иначе выбирать нечего.
	if len(available[0].Models) == 0 {
		t.Error("список моделей по умолчанию пуст")
	}

	if _, ok := cfg.Provider(ProviderGemini); !ok {
		t.Error("Provider(gemini) не найден")
	}
	if _, ok := cfg.Provider(ProviderClaude); ok {
		t.Error("Provider(claude) найден без ключа")
	}
}

func TestLoadConfigModelsOverride(t *testing.T) {
	t.Setenv("GEMINI_API_KEY", "test-key")
	t.Setenv("LLM_GEMINI_MODELS", " gemini-3-flash-preview , gemini-2.5-flash ,, ")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	provider, ok := cfg.Provider(ProviderGemini)
	if !ok {
		t.Fatal("провайдер не найден")
	}
	want := []string{"gemini-3-flash-preview", "gemini-2.5-flash"}
	if len(provider.Models) != len(want) {
		t.Fatalf("models = %v, ожидалось %v", provider.Models, want)
	}
	for i, model := range want {
		if provider.Models[i] != model {
			t.Errorf("models[%d] = %q, ожидалось %q", i, provider.Models[i], model)
		}
	}
	if provider.DefaultModel() != want[0] {
		t.Errorf("DefaultModel() = %q, ожидалось %q", provider.DefaultModel(), want[0])
	}
}

func TestLoadConfigInvalidValues(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
	}{
		{name: "битый таймаут", env: map[string]string{"LLM_TIMEOUT": "быстро"}},
		{name: "нулевой таймаут", env: map[string]string{"LLM_TIMEOUT": "0s"}},
		{name: "битый лимит символов", env: map[string]string{"LLM_MAX_PAGE_CHARS": "много"}},
		{name: "нулевой лимит символов", env: map[string]string{"LLM_MAX_PAGE_CHARS": "0"}},
		{name: "битая цена", env: map[string]string{"LLM_GEMINI_PRICE_INPUT": "дорого"}},
		{name: "отрицательная цена", env: map[string]string{"LLM_GEMINI_PRICE_OUTPUT": "-1"}},
		{name: "пустой список моделей", env: map[string]string{"LLM_GEMINI_MODELS": " , , "}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for key, value := range tc.env {
				t.Setenv(key, value)
			}

			// Плохая настройка должна валить старт, а не всплывать при первом запросе.
			if _, err := LoadConfig(); err == nil {
				t.Fatal("ожидалась ошибка конфигурации")
			}
		})
	}
}

func TestLoadConfigTimeouts(t *testing.T) {
	t.Setenv("LLM_TIMEOUT", "90s")
	t.Setenv("LLM_FETCH_TIMEOUT", "5s")
	t.Setenv("LLM_MAX_PAGE_CHARS", "1000")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}

	if cfg.Timeout != 90*time.Second {
		t.Errorf("Timeout = %v", cfg.Timeout)
	}
	if cfg.FetchTimeout != 5*time.Second {
		t.Errorf("FetchTimeout = %v", cfg.FetchTimeout)
	}
	if cfg.MaxPageChars != 1000 {
		t.Errorf("MaxPageChars = %d", cfg.MaxPageChars)
	}
}

func TestProviderHasModel(t *testing.T) {
	provider := ProviderConfig{
		ID:     ProviderGemini,
		APIKey: "test-key",
		Models: []string{"gemini-2.5-flash"},
	}

	if !provider.HasModel("gemini-2.5-flash") {
		t.Error("разрешённая модель не найдена")
	}
	// Клиент не должен подсунуть произвольную модель и потратить квоту неожиданно.
	if provider.HasModel("gemini-3.1-pro-preview") {
		t.Error("модель вне списка принята")
	}
	if provider.HasModel("") {
		t.Error("пустая модель принята")
	}
}

func TestPricingEstimate(t *testing.T) {
	t.Run("цена не задана", func(t *testing.T) {
		if got := (Pricing{}).Estimate(1000, 500); got != nil {
			t.Errorf("Estimate = %v, ожидался nil: показывать ноль было бы обманом", *got)
		}
	})

	t.Run("расчёт по прайсу", func(t *testing.T) {
		price := Pricing{InputPerMillion: 0.30, OutputPerMillion: 2.50}

		got := price.Estimate(1_000_000, 200_000)
		if got == nil {
			t.Fatal("Estimate = nil")
		}
		// 1M входных по 0.30 + 0.2M выходных по 2.50 = 0.30 + 0.50 = 0.80
		if diff := *got - 0.80; diff > 1e-9 || diff < -1e-9 {
			t.Errorf("Estimate = %v, ожидалось 0.80", *got)
		}
	})

	t.Run("известна только входная цена", func(t *testing.T) {
		price := Pricing{InputPerMillion: 1}
		if !price.Known() {
			t.Error("Known() = false")
		}
		if got := price.Estimate(1_000_000, 1_000_000); got == nil || *got != 1 {
			t.Errorf("Estimate = %v, ожидалось 1", got)
		}
	})
}

func TestProviderConfigNeverPrintsKey(t *testing.T) {
	const secret = "AQ.super-secret-key-value-12345"

	cfg := ProviderConfig{
		ID:     ProviderGemini,
		Label:  "Gemini",
		APIKey: secret,
		Models: []string{"gemini-2.5-flash"},
		Price:  Pricing{InputPerMillion: 0.3},
	}

	// Все формы вывода, которыми пользуются логи и сообщения об ошибках.
	outputs := map[string]string{
		"%v":            fmt.Sprintf("%v", cfg),
		"%+v":           fmt.Sprintf("%+v", cfg),
		"%#v":           fmt.Sprintf("%#v", cfg),
		"%s":            fmt.Sprintf("%s", cfg),
		"срез в %+v":    fmt.Sprintf("%+v", []ProviderConfig{cfg}),
		"внутри Config": fmt.Sprintf("%+v", Config{Providers: []ProviderConfig{cfg}}),
	}

	for form, output := range outputs {
		if strings.Contains(output, secret) {
			t.Errorf("%s печатает ключ: %s", form, output)
		}
		// При этом должно быть видно, что ключ вообще задан.
		if !strings.Contains(output, "скрыт") {
			t.Errorf("%s не показывает признак наличия ключа: %s", form, output)
		}
	}

	// Отсутствие ключа тоже должно быть различимо.
	empty := fmt.Sprintf("%v", ProviderConfig{ID: ProviderGemini})
	if !strings.Contains(empty, "не задан") {
		t.Errorf("вывод без ключа = %s", empty)
	}
}
