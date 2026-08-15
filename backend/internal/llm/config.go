package llm

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Значения по умолчанию. Модели выбраны из тех, что реально отдаёт провайдер;
// первой стоит более дешёвая, потому что она предвыбирается в интерфейсе.
const (
	defaultTimeout      = 60 * time.Second
	defaultFetchTimeout = 15 * time.Second
	defaultMaxPageChars = 40000
)

// Модели по умолчанию. Для Gemini обе проверены живым запросом: gemini-2.5-pro
// и gemini-2.5-flash-lite новым ключам больше не выдаются («no longer available
// to new users»), а pro-модели на бесплатном тарифе упираются в квоту.
var defaultModels = map[string][]string{
	ProviderGemini:   {"gemini-2.5-flash", "gemini-flash-latest"},
	ProviderClaude:   {"claude-sonnet-4-5", "claude-opus-4-1"},
	ProviderDeepSeek: {"deepseek-chat"},
}

// providerEnv описывает, из каких переменных окружения собирается провайдер.
var providerEnv = []struct {
	id       string
	label    string
	keyEnv   string
	envShort string
}{
	{id: ProviderGemini, label: "Gemini", keyEnv: "GEMINI_API_KEY", envShort: "GEMINI"},
	{id: ProviderClaude, label: "Claude", keyEnv: "ANTHROPIC_API_KEY", envShort: "CLAUDE"},
	{id: ProviderDeepSeek, label: "DeepSeek", keyEnv: "DEEPSEEK_API_KEY", envShort: "DEEPSEEK"},
}

// implementedProviders — провайдеры, для которых есть рабочий адаптер.
//
// Ключ без адаптера бесполезен: если показать такого провайдера в интерфейсе,
// пользователь выберет его и получит ошибку вместо результата. Поэтому такие
// провайдеры в конфигурацию не попадают, а о найденном ключе сообщается в лог.
var implementedProviders = map[string]bool{
	ProviderGemini: true,
}

// ImplementedProviders перечисляет провайдеров с готовым адаптером.
// Используется для сверки с реестром адаптеров, чтобы списки не разъехались.
func ImplementedProviders() []string {
	ids := make([]string, 0, len(implementedProviders))
	for _, spec := range providerEnv {
		if implementedProviders[spec.id] {
			ids = append(ids, spec.id)
		}
	}
	return ids
}

// Config — все настройки работы с моделями.
type Config struct {
	Providers []ProviderConfig
	// SkippedProviders — провайдеры, у которых есть ключ, но пока нет адаптера.
	// Нужны, чтобы сказать об этом при старте, а не молча игнорировать настройку.
	SkippedProviders []string
	// Timeout — таймаут одного запроса к модели.
	Timeout time.Duration
	// FetchTimeout — таймаут скачивания страницы вакансии.
	FetchTimeout time.Duration
	// MaxPageChars — сколько символов очищенного текста отдавать модели.
	MaxPageChars int
}

// LoadConfig читает настройки из окружения.
//
// Отсутствие ключей — не ошибка: без них приложение работает как на Этапе 1,
// просто заполнение через модель недоступно.
func LoadConfig() (Config, error) {
	cfg := Config{
		Timeout:      defaultTimeout,
		FetchTimeout: defaultFetchTimeout,
		MaxPageChars: defaultMaxPageChars,
	}

	var err error
	if cfg.Timeout, err = envDuration("LLM_TIMEOUT", defaultTimeout); err != nil {
		return Config{}, err
	}
	if cfg.FetchTimeout, err = envDuration("LLM_FETCH_TIMEOUT", defaultFetchTimeout); err != nil {
		return Config{}, err
	}
	if cfg.MaxPageChars, err = envPositiveInt("LLM_MAX_PAGE_CHARS", defaultMaxPageChars); err != nil {
		return Config{}, err
	}

	for _, spec := range providerEnv {
		apiKey := strings.TrimSpace(os.Getenv(spec.keyEnv))

		if !implementedProviders[spec.id] {
			if apiKey != "" {
				cfg.SkippedProviders = append(cfg.SkippedProviders, spec.id)
			}
			continue
		}

		provider := ProviderConfig{
			ID:     spec.id,
			Label:  spec.label,
			APIKey: apiKey,
			Models: defaultModels[spec.id],
		}

		if raw := os.Getenv("LLM_" + spec.envShort + "_MODELS"); raw != "" {
			models := parseModels(raw)
			if len(models) == 0 {
				return Config{}, fmt.Errorf("LLM_%s_MODELS=%q: список моделей пуст", spec.envShort, raw)
			}
			provider.Models = models
		}

		if provider.Price.InputPerMillion, err = envPrice("LLM_" + spec.envShort + "_PRICE_INPUT"); err != nil {
			return Config{}, err
		}
		if provider.Price.OutputPerMillion, err = envPrice("LLM_" + spec.envShort + "_PRICE_OUTPUT"); err != nil {
			return Config{}, err
		}

		cfg.Providers = append(cfg.Providers, provider)
	}

	return cfg, nil
}

// Available возвращает провайдеров, которыми можно пользоваться.
func (c Config) Available() []ProviderConfig {
	available := make([]ProviderConfig, 0, len(c.Providers))
	for _, provider := range c.Providers {
		if provider.Available() {
			available = append(available, provider)
		}
	}
	return available
}

// Enabled сообщает, доступен ли хотя бы один провайдер.
func (c Config) Enabled() bool {
	return len(c.Available()) > 0
}

// Provider ищет доступного провайдера по идентификатору.
func (c Config) Provider(id string) (ProviderConfig, bool) {
	for _, provider := range c.Providers {
		if provider.ID == id && provider.Available() {
			return provider, true
		}
	}
	return ProviderConfig{}, false
}

// parseModels разбирает список моделей через запятую, отбрасывая пустые элементы.
func parseModels(raw string) []string {
	parts := strings.Split(raw, ",")
	models := make([]string, 0, len(parts))
	for _, part := range parts {
		if model := strings.TrimSpace(part); model != "" {
			models = append(models, model)
		}
	}
	return models
}

func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}

	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("%s=%q: %w", key, raw, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("%s=%q: должен быть больше нуля", key, raw)
	}
	return value, nil
}

func envPositiveInt(key string, fallback int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s=%q: %w", key, raw, err)
	}
	if value < 1 {
		return 0, fmt.Errorf("%s=%q: должен быть не меньше 1", key, raw)
	}
	return value, nil
}

func envPrice(key string) (float64, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, nil
	}

	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, fmt.Errorf("%s=%q: ожидается число, цена за 1M токенов", key, raw)
	}
	if value < 0 {
		return 0, fmt.Errorf("%s=%q: цена не может быть отрицательной", key, raw)
	}
	return value, nil
}
