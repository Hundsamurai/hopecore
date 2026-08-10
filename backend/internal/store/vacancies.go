package store

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

// ErrNotFound возвращается, когда запись отсутствует. HTTP-слой превращает её в 404.
var ErrNotFound = errors.New("запись не найдена")

// ActiveFilterSQL повторяет правило activity.Resolve на стороне SQL:
// effective = manual ?? auto ?? true.
//
// Дублирование логики осознанное — фильтровать в БД дешевле, чем вычитывать всё
// и отбрасывать в Go. Согласованность двух реализаций закреплена тестом
// TestActiveFilterSQLMatchesResolve.
const ActiveFilterSQL = "COALESCE(manual_is_active, auto_is_active, 1) = 1"

// Поля, по которым разрешена сортировка списка. Белый список нужен потому,
// что имя колонки нельзя передать в запрос параметром.
var sortableColumns = map[string]string{
	"updated_at":  "updated_at",
	"created_at":  "created_at",
	"company":     "company",
	"opened_date": "opened_date",
	"grade":       "grade",
}

// SortableFields перечисляет допустимые значения параметра sort.
func SortableFields() []string {
	fields := make([]string, 0, len(sortableColumns))
	for name := range sortableColumns {
		fields = append(fields, name)
	}
	return fields
}

// IsSortableField сообщает, можно ли сортировать по этому полю.
func IsSortableField(name string) bool {
	_, ok := sortableColumns[name]
	return ok
}

// VacancyFilter описывает параметры выборки списка вакансий.
type VacancyFilter struct {
	// IncludeInactive снимает скрытие неактивных вакансий (п. 6 ТЗ).
	IncludeInactive bool
	// Sort — поле сортировки, по умолчанию updated_at.
	Sort string
	// Desc — порядок; по умолчанию по убыванию, то есть свежеизменённые сверху.
	Desc bool
}

// Normalize подставляет значения по умолчанию из ТЗ: сортировка по дате изменения,
// новые сверху.
func (f VacancyFilter) Normalize() VacancyFilter {
	if f.Sort == "" {
		f.Sort = "updated_at"
	}
	return f
}

// ListVacancies возвращает вакансии вместе со статусом кандидата:
// таблица в UI показывает дату отклика и этап собеседования.
func ListVacancies(db *gorm.DB, filter VacancyFilter) ([]model.Vacancy, error) {
	filter = filter.Normalize()

	column, ok := sortableColumns[filter.Sort]
	if !ok {
		return nil, fmt.Errorf("сортировка по полю %q не поддерживается", filter.Sort)
	}

	query := db.Preload("CandidateStatus")
	if !filter.IncludeInactive {
		query = query.Where(ActiveFilterSQL)
	}

	// Вторичная сортировка по id делает порядок стабильным, когда основной ключ совпадает.
	query = query.Order(clause.OrderByColumn{
		Column: clause.Column{Name: column},
		Desc:   filter.Desc,
	}).Order("id DESC")

	var vacancies []model.Vacancy
	if err := query.Find(&vacancies).Error; err != nil {
		return nil, fmt.Errorf("выборка вакансий: %w", err)
	}
	return vacancies, nil
}

// GetVacancy читает вакансию по id вместе со статусом кандидата.
func GetVacancy(db *gorm.DB, id uint) (*model.Vacancy, error) {
	var vacancy model.Vacancy
	err := db.Preload("CandidateStatus").First(&vacancy, id).Error
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return nil, ErrNotFound
	case err != nil:
		return nil, fmt.Errorf("чтение вакансии %d: %w", id, err)
	}
	return &vacancy, nil
}

// CreateVacancy сохраняет новую вакансию.
func CreateVacancy(db *gorm.DB, vacancy *model.Vacancy) error {
	// Omit(Associations) — на создании связей нет, а лишний upsert не нужен.
	if err := db.Omit(clause.Associations).Create(vacancy).Error; err != nil {
		return fmt.Errorf("создание вакансии: %w", err)
	}
	return nil
}

// SaveVacancy перезаписывает поля вакансии, не трогая связанные таблицы.
func SaveVacancy(db *gorm.DB, vacancy *model.Vacancy) error {
	if err := db.Omit(clause.Associations).Save(vacancy).Error; err != nil {
		return fmt.Errorf("сохранение вакансии %d: %w", vacancy.ID, err)
	}
	return nil
}

// DeleteVacancy удаляет вакансию. Статус кандидата, резюме собеседований
// и нейроблок уходят каскадом на уровне БД.
func DeleteVacancy(db *gorm.DB, id uint) error {
	result := db.Delete(&model.Vacancy{}, id)
	if result.Error != nil {
		return fmt.Errorf("удаление вакансии %d: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CheckableFilterSQL отбирает вакансии, которые имеет смысл опрашивать.
//
// Помеченные вручную как неактивные пропускаются: решение уже принято
// пользователем, а поход в сеть только тратит время и стучится на чужой сайт.
const CheckableFilterSQL = "manual_is_active IS NULL OR manual_is_active = 1"

// ListCheckableVacancies возвращает вакансии для массовой проверки активности.
func ListCheckableVacancies(db *gorm.DB) ([]model.Vacancy, error) {
	var vacancies []model.Vacancy
	err := db.Where(CheckableFilterSQL).Order("id ASC").Find(&vacancies).Error
	if err != nil {
		return nil, fmt.Errorf("выборка вакансий для проверки: %w", err)
	}
	return vacancies, nil
}

// SaveCheckResult записывает итог проверки активности.
//
// Используется UpdateColumns, а не Save: updated_at трогать нельзя. Иначе массовая
// проверка перетасовала бы всю таблицу, отсортированную по дате изменения,
// хотя пользователь ничего не менял.
func SaveCheckResult(db *gorm.DB, id uint, autoIsActive *bool, checkedAt time.Time, statusCode *int, checkErr string) error {
	columns := map[string]any{
		"last_checked_at":  checkedAt,
		"last_check_code":  statusCode,
		"last_check_error": checkErr,
	}
	// nil означает «результат неизвестен» — прежнее значение остаётся как было.
	if autoIsActive != nil {
		columns["auto_is_active"] = *autoIsActive
	}

	result := db.Model(&model.Vacancy{}).Where("id = ?", id).UpdateColumns(columns)
	if result.Error != nil {
		return fmt.Errorf("сохранение результата проверки вакансии %d: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SetManualActivity задаёт или снимает ручной override активности.
// nil снимает override — вакансия снова живёт по результату авто-проверки.
//
// Здесь updated_at обновляется: это осознанное действие пользователя,
// и вакансия должна подняться в списке.
func SetManualActivity(db *gorm.DB, id uint, manualIsActive *bool) error {
	result := db.Model(&model.Vacancy{}).Where("id = ?", id).
		Updates(map[string]any{"manual_is_active": manualIsActive})
	if result.Error != nil {
		return fmt.Errorf("обновление активности вакансии %d: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// CountVacancies возвращает общее число вакансий в базе.
func CountVacancies(db *gorm.DB) (int, error) {
	var count int64
	if err := db.Model(&model.Vacancy{}).Count(&count).Error; err != nil {
		return 0, fmt.Errorf("подсчёт вакансий: %w", err)
	}
	return int(count), nil
}
