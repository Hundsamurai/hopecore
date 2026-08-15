package store

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

// Ограничения выдачи журнала. Инструмент локальный, но отдавать всю историю
// одним ответом незачем: экран показывает последние запуски.
const (
	DefaultRunsLimit = 50
	MaxRunsLimit     = 200
)

// LLMRunFilter описывает выборку журнала.
type LLMRunFilter struct {
	// VacancyID ограничивает выборку одной вакансией: карточка показывает,
	// чем именно её заполняли.
	VacancyID *uint
	Limit     int
	Offset    int
}

// Normalize подставляет значения по умолчанию и режет слишком большой лимит.
func (f LLMRunFilter) Normalize() LLMRunFilter {
	if f.Limit <= 0 {
		f.Limit = DefaultRunsLimit
	}
	if f.Limit > MaxRunsLimit {
		f.Limit = MaxRunsLimit
	}
	if f.Offset < 0 {
		f.Offset = 0
	}
	return f
}

// LLMRunUsage — суммарный расход по журналу.
type LLMRunUsage struct {
	Runs         int
	InputTokens  int
	OutputTokens int
	// CostEstimate — сумма оценок; nil, если ни у одного запуска не было прайса.
	CostEstimate *float64
	// PricedRuns — по скольким запускам стоимость вообще известна.
	// Без этого сумма выглядела бы полной, хотя часть запусков в неё не вошла.
	PricedRuns int
}

// CreateLLMRun записывает запуск в журнал.
func CreateLLMRun(db *gorm.DB, run *model.LLMRun) error {
	if err := db.Omit(clause.Associations).Create(run).Error; err != nil {
		return fmt.Errorf("запись запуска модели: %w", err)
	}
	return nil
}

// ListLLMRuns возвращает запуски, свежие сверху, и общее число подходящих записей.
//
// Сырой ответ модели в выдачу не попадает: он бывает большим, а на экране
// журнала не нужен — только в карточке одного запуска.
func ListLLMRuns(db *gorm.DB, filter LLMRunFilter) ([]model.LLMRun, int, error) {
	filter = filter.Normalize()

	query := db.Model(&model.LLMRun{})
	if filter.VacancyID != nil {
		query = query.Where("vacancy_id = ?", *filter.VacancyID)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("подсчёт запусков модели: %w", err)
	}

	var runs []model.LLMRun
	err := query.
		Omit("response_json").
		Preload("Vacancy").
		Order("created_at DESC").
		Order("id DESC").
		Limit(filter.Limit).
		Offset(filter.Offset).
		Find(&runs).Error
	if err != nil {
		return nil, 0, fmt.Errorf("выборка запусков модели: %w", err)
	}

	return runs, int(total), nil
}

// GetLLMRun читает один запуск целиком, вместе с сырым ответом модели.
func GetLLMRun(db *gorm.DB, id uint) (*model.LLMRun, error) {
	var run model.LLMRun
	err := db.Preload("Vacancy").First(&run, id).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("чтение запуска модели %d: %w", id, err)
	}
	return &run, nil
}

// LLMRunsUsage считает суммарный расход по всему журналу.
func LLMRunsUsage(db *gorm.DB) (LLMRunUsage, error) {
	var row struct {
		Runs         int
		InputTokens  int
		OutputTokens int
		CostSum      *float64
		PricedRuns   int
	}

	err := db.Model(&model.LLMRun{}).
		Select(`COUNT(*) AS runs,
			COALESCE(SUM(input_tokens), 0) AS input_tokens,
			COALESCE(SUM(output_tokens), 0) AS output_tokens,
			SUM(cost_estimate) AS cost_sum,
			COUNT(cost_estimate) AS priced_runs`).
		Scan(&row).Error
	if err != nil {
		return LLMRunUsage{}, fmt.Errorf("подсчёт расхода по журналу: %w", err)
	}

	return LLMRunUsage{
		Runs:         row.Runs,
		InputTokens:  row.InputTokens,
		OutputTokens: row.OutputTokens,
		CostEstimate: row.CostSum,
		PricedRuns:   row.PricedRuns,
	}, nil
}
