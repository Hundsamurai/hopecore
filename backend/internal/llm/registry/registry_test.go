package registry

import (
	"testing"
	"time"

	"github.com/Hundsamurai/hopecore/backend/internal/llm"
)

// TestRegistryCoversImplementedProviders закрывает главный риск этой пары:
// список «реализованных» в llm и набор адаптеров здесь могут разъехаться,
// и тогда пользователь увидит провайдера, который не работает.
func TestRegistryCoversImplementedProviders(t *testing.T) {
	for _, id := range llm.ImplementedProviders() {
		cfg := llm.ProviderConfig{ID: id, APIKey: "test-key", Models: []string{"model"}}

		adapter, err := New(cfg, time.Second)
		if err != nil {
			t.Errorf("провайдер %q объявлен реализованным, но адаптера нет: %v", id, err)
			continue
		}
		if adapter.ID() != id {
			t.Errorf("адаптер вернул ID %q, ожидался %q", adapter.ID(), id)
		}
	}
}

func TestNewRequiresKey(t *testing.T) {
	_, err := New(llm.ProviderConfig{ID: llm.ProviderGemini}, time.Second)
	if err == nil {
		t.Fatal("адаптер создан без ключа")
	}
}

func TestNewRejectsUnknownProvider(t *testing.T) {
	_, err := New(llm.ProviderConfig{ID: "openai", APIKey: "test-key"}, time.Second)
	if err == nil {
		t.Fatal("создан адаптер для неизвестного провайдера")
	}
}

func TestBuildSkipsProvidersWithoutKey(t *testing.T) {
	cfg := llm.Config{
		Timeout: time.Second,
		Providers: []llm.ProviderConfig{
			{ID: llm.ProviderGemini, APIKey: "test-key", Models: []string{"gemini-2.5-flash"}},
		},
	}

	providers, err := Build(cfg)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(providers) != 1 || providers[llm.ProviderGemini] == nil {
		t.Errorf("собрано провайдеров: %d", len(providers))
	}

	// Без ключей набор пуст, и это не ошибка.
	empty, err := Build(llm.Config{Timeout: time.Second})
	if err != nil {
		t.Fatalf("Build без провайдеров: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("собрано %d провайдеров без ключей", len(empty))
	}
}
