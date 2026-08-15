package api

import (
	"math"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/Hundsamurai/hopecore/backend/internal/activity"
	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

// maxTextLen ограничивает длину текстовых полей. Защита не от злоумышленника
// (инструмент локальный), а от случайной вставки всей страницы в поле компании.
const maxTextLen = 2000

// vacancyResponse — представление вакансии в API.
//
// Активность отдаётся сразу в трёх видах: вычисленное is_active для отображения,
// плюс исходные auto/manual — чтобы UI мог показать, откуда значение взялось,
// и предложить сбросить override.
type vacancyResponse struct {
	ID         uint        `json:"id"`
	URL        string      `json:"url"`
	Title      string      `json:"title"`
	Company    string      `json:"company"`
	Grade      string      `json:"grade"`
	TechTags   model.Tags  `json:"tech_tags"`
	OpenedDate *model.Date `json:"opened_date"`

	SalaryFrom     *float64 `json:"salary_from"`
	SalaryTo       *float64 `json:"salary_to"`
	SalaryCurrency string   `json:"salary_currency"`
	SalaryGross    *bool    `json:"salary_gross"`
	WorkFormat     string   `json:"work_format"`

	IsActive         bool  `json:"is_active"`
	ActivityConflict bool  `json:"activity_conflict"`
	AutoIsActive     *bool `json:"auto_is_active"`
	ManualIsActive   *bool `json:"manual_is_active"`

	LastCheckedAt  *time.Time `json:"last_checked_at"`
	LastCheckCode  *int       `json:"last_check_code"`
	LastCheckError string     `json:"last_check_error"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	CandidateStatus *candidateStatusResponse `json:"candidate_status"`
}

// candidateStatusResponse — статус кандидата по вакансии (п. 4.2 ТЗ).
// CRUD появляется в Task 6, но в ответе поле есть уже сейчас:
// таблица вакансий показывает дату отклика и этап собеседования.
type candidateStatusResponse struct {
	ID                 uint        `json:"id"`
	VacancyID          uint        `json:"vacancy_id"`
	CoverLetter        string      `json:"cover_letter"`
	SentAt             *model.Date `json:"sent_at"`
	InterviewStage     string      `json:"interview_stage"`
	HRContact          string      `json:"hr_contact"`
	InterviewRecordURL string      `json:"interview_record_url"`
	OfferReceived      bool        `json:"offer_received"`
	OfferedSalary      *float64    `json:"offered_salary"`
	RealSalary         *float64    `json:"real_salary"`
	MarketSalaryData   string      `json:"market_salary_data"`
	CreatedAt          time.Time   `json:"created_at"`
	UpdatedAt          time.Time   `json:"updated_at"`
}

func newVacancyResponse(v model.Vacancy) vacancyResponse {
	isActive, conflict := activity.Resolve(v.AutoIsActive, v.ManualIsActive)

	resp := vacancyResponse{
		ID:               v.ID,
		URL:              v.URL,
		Title:            v.Title,
		Company:          v.Company,
		Grade:            v.Grade,
		TechTags:         v.TechTags,
		OpenedDate:       v.OpenedDate,
		SalaryFrom:       v.SalaryFrom,
		SalaryTo:         v.SalaryTo,
		SalaryCurrency:   v.SalaryCurrency,
		SalaryGross:      v.SalaryGross,
		WorkFormat:       v.WorkFormat,
		IsActive:         isActive,
		ActivityConflict: conflict,
		AutoIsActive:     v.AutoIsActive,
		ManualIsActive:   v.ManualIsActive,
		LastCheckedAt:    v.LastCheckedAt,
		LastCheckCode:    v.LastCheckCode,
		LastCheckError:   v.LastCheckError,
		CreatedAt:        v.CreatedAt,
		UpdatedAt:        v.UpdatedAt,
	}

	if v.CandidateStatus != nil {
		status := newCandidateStatusResponse(*v.CandidateStatus)
		resp.CandidateStatus = &status
	}
	return resp
}

func newCandidateStatusResponse(s model.CandidateStatus) candidateStatusResponse {
	return candidateStatusResponse{
		ID:                 s.ID,
		VacancyID:          s.VacancyID,
		CoverLetter:        s.CoverLetter,
		SentAt:             s.SentAt,
		InterviewStage:     s.InterviewStage,
		HRContact:          s.HRContact,
		InterviewRecordURL: s.InterviewRecordURL,
		OfferReceived:      s.OfferReceived,
		OfferedSalary:      s.OfferedSalary,
		RealSalary:         s.RealSalary,
		MarketSalaryData:   s.MarketSalaryData,
		CreatedAt:          s.CreatedAt,
		UpdatedAt:          s.UpdatedAt,
	}
}

func newVacancyListResponse(vacancies []model.Vacancy) []vacancyResponse {
	items := make([]vacancyResponse, 0, len(vacancies))
	for _, v := range vacancies {
		items = append(items, newVacancyResponse(v))
	}
	return items
}

// createVacancyRequest — тело POST /api/vacancies.
// Обязателен только url: остальное кандидат заполняет по мере появления информации.
type createVacancyRequest struct {
	URL        string      `json:"url"`
	Title      string      `json:"title"`
	Company    string      `json:"company"`
	Grade      string      `json:"grade"`
	TechTags   []string    `json:"tech_tags"`
	OpenedDate *model.Date `json:"opened_date"`

	SalaryFrom     *float64 `json:"salary_from"`
	SalaryTo       *float64 `json:"salary_to"`
	SalaryCurrency string   `json:"salary_currency"`
	SalaryGross    *bool    `json:"salary_gross"`
	WorkFormat     string   `json:"work_format"`
}

func (r createVacancyRequest) validate() fieldErrors {
	errs := fieldErrors{}

	if msg := validateURL(r.URL); msg != "" {
		errs.add("url", msg)
	}
	if !model.IsValidGrade(r.Grade) {
		errs.add("grade", gradeErrorMessage())
	}
	if !model.IsValidWorkFormat(r.WorkFormat) {
		errs.add("work_format", workFormatErrorMessage())
	}
	for field, value := range map[string]string{"title": r.Title, "company": r.Company} {
		if len(value) > maxTextLen {
			errs.add(field, "слишком длинное значение")
		}
	}
	if msg := validateTags(r.TechTags); msg != "" {
		errs.add("tech_tags", msg)
	}
	if msg := validateSalaryBound(r.SalaryFrom); msg != "" {
		errs.add("salary_from", msg)
	}
	if msg := validateSalaryBound(r.SalaryTo); msg != "" {
		errs.add("salary_to", msg)
	}
	if msg := validateCurrency(r.SalaryCurrency); msg != "" {
		errs.add("salary_currency", msg)
	}
	return errs
}

func (r createVacancyRequest) toModel() model.Vacancy {
	return model.Vacancy{
		URL:            strings.TrimSpace(r.URL),
		Title:          strings.TrimSpace(r.Title),
		Company:        strings.TrimSpace(r.Company),
		Grade:          r.Grade,
		TechTags:       normalizeTags(r.TechTags),
		OpenedDate:     r.OpenedDate,
		SalaryFrom:     r.SalaryFrom,
		SalaryTo:       r.SalaryTo,
		SalaryCurrency: normalizeCurrency(r.SalaryCurrency),
		SalaryGross:    r.SalaryGross,
		WorkFormat:     r.WorkFormat,
	}
}

// updateVacancyRequest — тело PATCH /api/vacancies/{id}.
// Каждое поле опционально: отсутствие означает «не трогать», null — «очистить».
type updateVacancyRequest struct {
	URL        Optional[string]     `json:"url"`
	Title      Optional[string]     `json:"title"`
	Company    Optional[string]     `json:"company"`
	Grade      Optional[string]     `json:"grade"`
	TechTags   Optional[[]string]   `json:"tech_tags"`
	OpenedDate Optional[model.Date] `json:"opened_date"`

	SalaryFrom     Optional[float64] `json:"salary_from"`
	SalaryTo       Optional[float64] `json:"salary_to"`
	SalaryCurrency Optional[string]  `json:"salary_currency"`
	SalaryGross    Optional[bool]    `json:"salary_gross"`
	WorkFormat     Optional[string]  `json:"work_format"`
}

func (r updateVacancyRequest) validate() fieldErrors {
	errs := fieldErrors{}

	if r.URL.Cleared() {
		errs.add("url", "ссылку нельзя очистить")
	}
	if r.URL.Provided() {
		if msg := validateURL(*r.URL.Value); msg != "" {
			errs.add("url", msg)
		}
	}
	if r.Grade.Provided() && !model.IsValidGrade(*r.Grade.Value) {
		errs.add("grade", gradeErrorMessage())
	}
	if r.WorkFormat.Provided() && !model.IsValidWorkFormat(*r.WorkFormat.Value) {
		errs.add("work_format", workFormatErrorMessage())
	}
	if r.Title.Provided() && len(*r.Title.Value) > maxTextLen {
		errs.add("title", "слишком длинное значение")
	}
	if r.Company.Provided() && len(*r.Company.Value) > maxTextLen {
		errs.add("company", "слишком длинное значение")
	}
	if r.TechTags.Provided() {
		if msg := validateTags(*r.TechTags.Value); msg != "" {
			errs.add("tech_tags", msg)
		}
	}
	if r.SalaryFrom.Provided() {
		if msg := validateSalaryBound(r.SalaryFrom.Value); msg != "" {
			errs.add("salary_from", msg)
		}
	}
	if r.SalaryTo.Provided() {
		if msg := validateSalaryBound(r.SalaryTo.Value); msg != "" {
			errs.add("salary_to", msg)
		}
	}
	if r.SalaryCurrency.Provided() {
		if msg := validateCurrency(*r.SalaryCurrency.Value); msg != "" {
			errs.add("salary_currency", msg)
		}
	}
	return errs
}

// apply переносит присланные поля в существующую запись.
func (r updateVacancyRequest) apply(v *model.Vacancy) {
	if r.URL.Provided() {
		v.URL = strings.TrimSpace(*r.URL.Value)
	}

	if r.Company.Set {
		if r.Company.Value == nil {
			v.Company = ""
		} else {
			v.Company = strings.TrimSpace(*r.Company.Value)
		}
	}

	if r.Grade.Set {
		if r.Grade.Value == nil {
			v.Grade = ""
		} else {
			v.Grade = *r.Grade.Value
		}
	}

	if r.TechTags.Set {
		if r.TechTags.Value == nil {
			v.TechTags = model.Tags{}
		} else {
			v.TechTags = normalizeTags(*r.TechTags.Value)
		}
	}

	if r.OpenedDate.Set {
		v.OpenedDate = r.OpenedDate.Value
	}

	if r.Title.Set {
		if r.Title.Value == nil {
			v.Title = ""
		} else {
			v.Title = strings.TrimSpace(*r.Title.Value)
		}
	}

	if r.SalaryFrom.Set {
		v.SalaryFrom = r.SalaryFrom.Value
	}
	if r.SalaryTo.Set {
		v.SalaryTo = r.SalaryTo.Value
	}
	if r.SalaryGross.Set {
		v.SalaryGross = r.SalaryGross.Value
	}

	if r.SalaryCurrency.Set {
		if r.SalaryCurrency.Value == nil {
			v.SalaryCurrency = ""
		} else {
			v.SalaryCurrency = normalizeCurrency(*r.SalaryCurrency.Value)
		}
	}

	if r.WorkFormat.Set {
		if r.WorkFormat.Value == nil {
			v.WorkFormat = ""
		} else {
			v.WorkFormat = *r.WorkFormat.Value
		}
	}
}

// normalizeSalary приводит зарплатные поля вакансии в согласованное состояние
// и проверяет то, что нельзя проверить по одному полю.
//
// Вызывается после сборки или правки записи, а не на уровне запроса: при PATCH
// одна граница может прийти в теле, а вторая остаться прежней, и сравнивать
// нужно итоговые значения.
func normalizeSalary(v *model.Vacancy) fieldErrors {
	errs := fieldErrors{}

	if v.SalaryFrom != nil && v.SalaryTo != nil && *v.SalaryFrom > *v.SalaryTo {
		errs.add("salary_to", "верхняя граница вилки не может быть меньше нижней")
		return errs
	}

	hasSalary := v.SalaryFrom != nil || v.SalaryTo != nil

	switch {
	case hasSalary && v.SalaryCurrency == "":
		// Вилка без валюты бессмысленна, а в объявлениях по умолчанию рубли.
		v.SalaryCurrency = model.DefaultSalaryCurrency
	case !hasSalary:
		// Валюта и признак «до вычета налогов» без вилки ничего не означают.
		v.SalaryCurrency = ""
		v.SalaryGross = nil
	}

	return errs
}

func validateSalaryBound(value *float64) string {
	if value == nil {
		return ""
	}
	if math.IsNaN(*value) || math.IsInf(*value, 0) {
		return "ожидается число"
	}
	if *value < 0 {
		return "зарплата не может быть отрицательной"
	}
	if *value > model.MaxSalary {
		return "неправдоподобно большое значение"
	}
	return ""
}

// currencyPattern — код валюты по ISO 4217: три латинские буквы.
var currencyPattern = regexp.MustCompile(`^[A-Za-z]{3}$`)

func validateCurrency(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if !currencyPattern.MatchString(trimmed) {
		return "ожидается код валюты из трёх латинских букв, например RUB"
	}
	return ""
}

// normalizeCurrency приводит код к верхнему регистру: «rub» и «RUB» — одно и то же.
func normalizeCurrency(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func workFormatErrorMessage() string {
	return "ожидается одно из значений: " + strings.Join(model.WorkFormats, ", ")
}

func validateURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "обязательное поле"
	}
	if len(raw) > maxTextLen {
		return "слишком длинная ссылка"
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "не удалось разобрать ссылку"
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "ожидается ссылка со схемой http или https"
	}
	if parsed.Host == "" {
		return "в ссылке не указан домен"
	}
	return ""
}

func validateTags(tags []string) string {
	if len(tags) > 50 {
		return "слишком много тегов, максимум 50"
	}
	for _, tag := range tags {
		if len(tag) > 100 {
			return "тег длиннее 100 символов"
		}
	}
	return ""
}

// normalizeTags убирает пробелы и пустые значения, сохраняя порядок:
// он несёт смысл (сначала главные технологии).
func normalizeTags(tags []string) model.Tags {
	result := make(model.Tags, 0, len(tags))
	for _, tag := range tags {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			continue
		}
		result = append(result, trimmed)
	}
	return result
}

func gradeErrorMessage() string {
	return "ожидается одно из значений: " + strings.Join(model.Grades, ", ")
}
