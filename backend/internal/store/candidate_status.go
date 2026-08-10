package store

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

// UpsertCandidateStatus создаёт или полностью перезаписывает статус кандидата.
//
// Связь с вакансией 1:1 (п. 4.2 ТЗ), поэтому отдельного создания и обновления нет:
// клиент присылает состояние формы, а сервер приводит запись к нему.
//
// Побочный эффект: у вакансии обновляется updated_at. Для пользователя заполнение
// статуса — это правка карточки, и вакансия должна подняться в списке,
// отсортированном по дате изменения. Этим upsert отличается от проверки
// активности, которая updated_at не трогает.
func UpsertCandidateStatus(db *gorm.DB, status *model.CandidateStatus) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var vacancies int64
		if err := tx.Model(&model.Vacancy{}).Where("id = ?", status.VacancyID).Count(&vacancies).Error; err != nil {
			return fmt.Errorf("проверка вакансии %d: %w", status.VacancyID, err)
		}
		if vacancies == 0 {
			return ErrNotFound
		}

		var existing model.CandidateStatus
		err := tx.Where("vacancy_id = ?", status.VacancyID).First(&existing).Error

		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			if err := tx.Omit(clause.Associations).Create(status).Error; err != nil {
				return fmt.Errorf("создание статуса кандидата: %w", err)
			}
		case err != nil:
			return fmt.Errorf("чтение статуса кандидата: %w", err)
		default:
			// Сохраняем идентификатор и дату создания: запись та же, меняется содержимое.
			status.ID = existing.ID
			status.CreatedAt = existing.CreatedAt
			if err := tx.Omit(clause.Associations).Save(status).Error; err != nil {
				return fmt.Errorf("обновление статуса кандидата: %w", err)
			}
		}

		if err := tx.Model(&model.Vacancy{}).Where("id = ?", status.VacancyID).
			UpdateColumn("updated_at", time.Now().UTC()).Error; err != nil {
			return fmt.Errorf("обновление даты изменения вакансии %d: %w", status.VacancyID, err)
		}
		return nil
	})
}

// GetCandidateStatus читает статус кандидата по вакансии.
func GetCandidateStatus(db *gorm.DB, vacancyID uint) (*model.CandidateStatus, error) {
	var status model.CandidateStatus
	err := db.Where("vacancy_id = ?", vacancyID).First(&status).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("чтение статуса кандидата вакансии %d: %w", vacancyID, err)
	}
	return &status, nil
}
