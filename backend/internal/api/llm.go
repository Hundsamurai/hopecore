package api

import (
	"net/http"

	"github.com/Hundsamurai/hopecore/backend/internal/llm"
)

// providerResponse — провайдер в том виде, в каком его видит клиент.
// Ключа здесь нет и быть не может: наружу уходят только идентификатор,
// человеческое название и список разрешённых моделей.
type providerResponse struct {
	ID           string   `json:"id"`
	Label        string   `json:"label"`
	Models       []string `json:"models"`
	DefaultModel string   `json:"default_model"`
	// PriceKnown говорит интерфейсу, показывать ли оценку стоимости
	// или ограничиться токенами.
	PriceKnown bool `json:"price_known"`
}

type providersResponse struct {
	Items []providerResponse `json:"items"`
}

// handleListProviders отдаёт провайдеров, которыми можно пользоваться.
//
// Пустой список — не ошибка: значит ключей нет, и приложение работает
// как на Этапе 1. Интерфейс по этому признаку выключает кнопку заполнения.
func (s *Server) handleListProviders(w http.ResponseWriter, _ *http.Request) {
	available := s.llm.Available()

	items := make([]providerResponse, 0, len(available))
	for _, provider := range available {
		items = append(items, newProviderResponse(provider))
	}

	writeJSON(w, http.StatusOK, providersResponse{Items: items})
}

func newProviderResponse(provider llm.ProviderConfig) providerResponse {
	return providerResponse{
		ID:           provider.ID,
		Label:        provider.Label,
		Models:       provider.Models,
		DefaultModel: provider.DefaultModel(),
		PriceKnown:   provider.Price.Known(),
	}
}
