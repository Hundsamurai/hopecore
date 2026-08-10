package model

import "time"

// CandidateStatus — статус прохождения кандидатом вакансии (п. 4.2 ТЗ).
// Связь с вакансией строго 1:1, поэтому на VacancyID стоит уникальный индекс.
type CandidateStatus struct {
	ID        uint `gorm:"primaryKey"`
	VacancyID uint `gorm:"not null;uniqueIndex"`

	CoverLetter        string
	SentAt             *Date
	InterviewStage     string
	HRContact          string
	InterviewRecordURL string
	OfferReceived      bool
	OfferedSalary      *float64
	RealSalary         *float64
	MarketSalaryData   string

	CreatedAt time.Time
	UpdatedAt time.Time

	// Резюме собеседований приходят из внешних систем (п. 4.3 ТЗ).
	// В MVP таблица только создаётся миграцией, API её не трогает.
	InterviewSummaries []InterviewSummary `gorm:"constraint:OnDelete:CASCADE"`
}

// TableName закрепляет имя таблицы из дизайн-документа.
func (CandidateStatus) TableName() string {
	return "candidate_status"
}
