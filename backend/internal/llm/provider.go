// Package llm описывает провайдеров языковых моделей, их настройки
// и общую обвязку: схему извлечения, валидацию и оценку стоимости.
//
// Пакет ничего не знает про БД и про HTTP-слой приложения: оркестрация живёт
// в internal/service, как и у проверки активности.
package llm

import "fmt"

// Идентификаторы провайдеров. Значения попадают в API и в журнал запусков,
// поэтому меняться не должны.
const (
	ProviderGemini   = "gemini"
	ProviderClaude   = "claude"
	ProviderDeepSeek = "deepseek"
)

// Pricing — цена за миллион токенов. Задаётся в конфигурации и может отсутствовать:
// цены провайдеров меняются, и устаревшая цифра хуже честного «неизвестно».
type Pricing struct {
	InputPerMillion  float64
	OutputPerMillion float64
}

// Known сообщает, задана ли цена хотя бы частично.
func (p Pricing) Known() bool {
	return p.InputPerMillion > 0 || p.OutputPerMillion > 0
}

// Estimate считает ориентировочную стоимость запроса.
// Возвращает nil, если цена не задана: показывать ноль было бы обманом.
func (p Pricing) Estimate(inputTokens, outputTokens int) *float64 {
	if !p.Known() {
		return nil
	}
	cost := float64(inputTokens)/1_000_000*p.InputPerMillion +
		float64(outputTokens)/1_000_000*p.OutputPerMillion
	return &cost
}

// ProviderConfig — настройки одного провайдера.
//
// APIKey сознательно не имеет json-тега и никогда не сериализуется:
// наружу уходит только идентификатор, название и список моделей.
type ProviderConfig struct {
	ID     string
	Label  string
	APIKey string
	Models []string
	Price  Pricing
}

// Available сообщает, можно ли пользоваться провайдером: нужен и ключ, и модели.
func (c ProviderConfig) Available() bool {
	return c.APIKey != "" && len(c.Models) > 0
}

// String скрывает ключ при выводе.
//
// Метод появился после реального случая: упавший тест напечатал структуру
// через %+v вместе с настоящим ключом. Пока ключ — обычное поле, он утечёт
// в первом же сообщении об ошибке или отладочном логе; со Stringer это
// перестаёт быть возможным.
func (c ProviderConfig) String() string {
	return fmt.Sprintf("{ID:%s Label:%s APIKey:%s Models:%v Price:%+v}",
		c.ID, c.Label, redact(c.APIKey), c.Models, c.Price)
}

// GoString закрывает и вывод через %#v.
func (c ProviderConfig) GoString() string {
	return c.String()
}

// redact оставляет только признак наличия ключа и его длину: этого хватает,
// чтобы отличить «ключа нет» от «ключ не тот», и недостаточно, чтобы им
// воспользоваться.
func redact(key string) string {
	if key == "" {
		return "<не задан>"
	}
	return fmt.Sprintf("<скрыт, %d символов>", len(key))
}

// HasModel проверяет, что модель разрешена конфигурацией. Нужен, чтобы клиент
// не мог подсунуть произвольное имя модели и потратить квоту неожиданным образом.
func (c ProviderConfig) HasModel(model string) bool {
	for _, allowed := range c.Models {
		if allowed == model {
			return true
		}
	}
	return false
}

// DefaultModel — первая модель из списка. Она предвыбирается в интерфейсе,
// поэтому в конфигурации первой стоит ставить самую дешёвую.
func (c ProviderConfig) DefaultModel() string {
	if len(c.Models) == 0 {
		return ""
	}
	return c.Models[0]
}

// Интерфейс Provider и типы запроса-ответа появятся вместе со схемой
// извлечения: описывать их до неё нечестно, форма запроса зависит от схемы.
