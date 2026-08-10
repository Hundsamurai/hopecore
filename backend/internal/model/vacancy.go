// Package model описывает сущности трекера вакансий и их отображение в SQLite.
package model

import "time"

// Grade — грейд вакансии. Допустимые значения перечислены явно,
// чтобы валидация в API и подсказки в UI брались из одного места.
const (
	GradeIntern = "intern"
	GradeJunior = "junior"
	GradeMiddle = "middle"
	GradeSenior = "senior"
	GradeLead   = "lead"
)

// Grades — все допустимые грейды. Пустой грейд тоже валиден: вакансия может быть
// добавлена по ссылке до того, как кандидат разобрался с уровнем.
var Grades = []string{GradeIntern, GradeJunior, GradeMiddle, GradeSenior, GradeLead}

// IsValidGrade сообщает, входит ли значение в допустимый набор.
// Пустая строка считается валидной (грейд не указан).
func IsValidGrade(grade string) bool {
	if grade == "" {
		return true
	}
	for _, g := range Grades {
		if g == grade {
			return true
		}
	}
	return false
}

// Vacancy — карточка вакансии (п. 4.1 ТЗ).
//
// Про активность: в ТЗ было одно поле is_active, но оно не позволяет одновременно
// хранить результат авто-проверки и решение пользователя. Поэтому состояние
// расщеплено на AutoIsActive и ManualIsActive, а отображаемое значение вычисляется
// (см. docs/main/design-stage1.md, п. 6 и п. 9). Оба поля nullable:
//   - AutoIsActive == nil   — проверка ещё не дала определённого ответа;
//   - ManualIsActive == nil — пользователь не переопределял активность.
type Vacancy struct {
	ID       uint   `gorm:"primaryKey"`
	URL      string `gorm:"not null;index"`
	Company  string
	Grade    string
	TechTags Tags
	// OpenedDate — дата открытия вакансии, заполняется вручную.
	OpenedDate *Date

	AutoIsActive   *bool
	ManualIsActive *bool

	// LastCheckedAt — когда пользователь последний раз вручную запускал проверку.
	// Крона в MVP нет, поэтому смысл поля отличается от формулировки в ТЗ (п. 9 дизайна).
	LastCheckedAt  *time.Time
	LastCheckCode  *int
	LastCheckError string

	CreatedAt time.Time
	UpdatedAt time.Time

	// Связи. OnDelete:CASCADE вешается на внешние ключи при миграции,
	// работает благодаря включённому pragma foreign_keys.
	CandidateStatus *CandidateStatus `gorm:"constraint:OnDelete:CASCADE"`
	AIBlock         *AIBlock         `gorm:"constraint:OnDelete:CASCADE"`
}

// TableName закрепляет имя таблицы из дизайн-документа вместо автогенерации gorm.
func (Vacancy) TableName() string {
	return "vacancies"
}
