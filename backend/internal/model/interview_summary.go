package model

import "time"

// InterviewSummary — резюме пройденного собеседования (п. 4.3 ТЗ).
//
// Само резюме и оценки формируются внешними системами, трекер только хранит
// и отображает результат. В MVP (Этап 1) таблица создаётся миграцией,
// но ни API, ни UI с ней не работают — это Этап 3.
type InterviewSummary struct {
	ID                uint `gorm:"primaryKey"`
	CandidateStatusID uint `gorm:"not null;index"`

	RecordURL string
	// AIScore и оценки людей хранятся текстом: в ТЗ формат не зафиксирован
	// ("numeric/text"), а источники внешние и могут присылать что угодно.
	AIScore            string
	ProctoringUsed     bool
	ReviewerScore      string
	CandidateSelfScore string

	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName закрепляет имя таблицы из дизайн-документа.
func (InterviewSummary) TableName() string {
	return "interview_summary"
}
