// Package registry собирает адаптеры провайдеров по конфигурации.
//
// Пакет отдельный, потому что адаптеры импортируют llm (за типами схемы
// и запроса), и обратный импорт дал бы цикл.
package registry

import (
	"fmt"
	"time"

	"github.com/Hundsamurai/hopecore/backend/internal/llm"
	"github.com/Hundsamurai/hopecore/backend/internal/llm/gemini"
)

// New создаёт адаптер по настройкам провайдера.
func New(cfg llm.ProviderConfig, timeout time.Duration) (llm.Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("для провайдера %s не задан ключ", cfg.ID)
	}

	switch cfg.ID {
	case llm.ProviderGemini:
		return gemini.New(cfg.APIKey, timeout), nil
	default:
		// Сюда попасть нельзя: llm.LoadConfig не отдаёт провайдеров без адаптера.
		// Проверка оставлена как страховка от расхождения списков.
		return nil, fmt.Errorf("для провайдера %s нет адаптера", cfg.ID)
	}
}

// Build собирает адаптеры для всех доступных провайдеров.
func Build(cfg llm.Config) (map[string]llm.Provider, error) {
	providers := make(map[string]llm.Provider)

	for _, provider := range cfg.Available() {
		adapter, err := New(provider, cfg.Timeout)
		if err != nil {
			return nil, err
		}
		providers[provider.ID] = adapter
	}
	return providers, nil
}
