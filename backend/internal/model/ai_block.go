package model

import "time"

// AIBlock — нейроблок вакансии (п. 4.4 ТЗ): прогноз ЗП и оценка соответствия рынку.
//
// В MVP таблица создаётся миграцией, но не используется: заполнение по кнопке
// с выбором LLM-провайдера — это Этап 3.
type AIBlock struct {
	ID        uint `gorm:"primaryKey"`
	VacancyID uint `gorm:"not null;uniqueIndex"`

	SalaryForecast string
	MarketMatch    string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName закрепляет имя таблицы из дизайн-документа.
func (AIBlock) TableName() string {
	return "ai_block"
}
