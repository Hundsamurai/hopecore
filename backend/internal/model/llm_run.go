package model

import "time"

// Назначение запуска. Пока одно; на Этапе 3 добавится нейроблок.
const (
	PurposeExtractVacancy = "extract_vacancy"
)

// Итоги запуска. Различаются подробно, чтобы в журнале была видна причина,
// а не общее «не получилось».
const (
	// RunStatusOK — модель ответила, ответ разобран и провалидирован.
	RunStatusOK = "ok"
	// RunStatusFetchError — страницу не удалось скачать; токены не потрачены.
	RunStatusFetchError = "fetch_error"
	// RunStatusProviderError — провайдер вернул ошибку (квота, недоступность).
	RunStatusProviderError = "provider_error"
	// RunStatusInvalidJSON — ответ не удалось привести к схеме даже со второй попытки.
	RunStatusInvalidJSON = "invalid_json"
	// RunStatusTimeout — модель или страница не ответили за отведённое время.
	RunStatusTimeout = "timeout"
)

// LLMRun — запись журнала обращений к языковой модели.
//
// Журнал нужен для двух вещей: видеть, куда уходит квота, и понимать
// происхождение данных в карточке — чем и когда поле было заполнено.
type LLMRun struct {
	ID        uint `gorm:"primaryKey"`
	CreatedAt time.Time

	Purpose string `gorm:"not null;index"`

	// VacancyID nullable: при удалении вакансии внешний ключ обнуляется,
	// но запись о трате остаётся. История расходов не должна исчезать
	// вместе с карточкой.
	VacancyID *uint `gorm:"index"`

	Provider string `gorm:"not null"`
	Model    string `gorm:"not null"`
	// PromptVersion позволяет объяснить старый результат: без неё непонятно,
	// каким промптом он получен.
	PromptVersion string

	SourceURL string
	// SourceChars — сколько символов текста реально ушло в модель.
	// По нему видно, была ли страница пустой.
	SourceChars int

	Status string `gorm:"not null;index"`

	// Счётчики токенов nullable: провайдер может их не вернуть, и ноль
	// в этом случае был бы неправдой.
	InputTokens  *int
	OutputTokens *int
	// CostEstimate — оценка по прайсу из конфигурации; nil, если прайс не задан.
	CostEstimate *float64

	// Attempts — сколько вызовов модели потребовалось. Больше одного означает,
	// что первый ответ не прошёл валидацию (характерно для DeepSeek).
	Attempts   int
	DurationMs int
	Error      string

	// ResponseJSON — сырой ответ модели. Хранится, чтобы можно было посмотреть,
	// что она сказала, когда результат выглядит странно.
	ResponseJSON string

	// Vacancy подгружается только для чтения: на экране журнала нужно название
	// вакансии, а не один идентификатор. Может быть nil — вакансию могли удалить.
	Vacancy *Vacancy `gorm:"foreignKey:VacancyID"`
}

// TableName закрепляет имя таблицы из дизайн-документа.
func (LLMRun) TableName() string {
	return "llm_runs"
}
