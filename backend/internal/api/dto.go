package api

import (
	"net/url"
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
	Company    string      `json:"company"`
	Grade      string      `json:"grade"`
	TechTags   model.Tags  `json:"tech_tags"`
	OpenedDate *model.Date `json:"opened_date"`

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
		Company:          v.Company,
		Grade:            v.Grade,
		TechTags:         v.TechTags,
		OpenedDate:       v.OpenedDate,
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
	Company    string      `json:"company"`
	Grade      string      `json:"grade"`
	TechTags   []string    `json:"tech_tags"`
	OpenedDate *model.Date `json:"opened_date"`
}

func (r createVacancyRequest) validate() fieldErrors {
	errs := fieldErrors{}

	if msg := validateURL(r.URL); msg != "" {
		errs.add("url", msg)
	}
	if !model.IsValidGrade(r.Grade) {
		errs.add("grade", gradeErrorMessage())
	}
	if len(r.Company) > maxTextLen {
		errs.add("company", "слишком длинное значение")
	}
	if msg := validateTags(r.TechTags); msg != "" {
		errs.add("tech_tags", msg)
	}
	return errs
}

func (r createVacancyRequest) toModel() model.Vacancy {
	return model.Vacancy{
		URL:        strings.TrimSpace(r.URL),
		Company:    strings.TrimSpace(r.Company),
		Grade:      r.Grade,
		TechTags:   normalizeTags(r.TechTags),
		OpenedDate: r.OpenedDate,
	}
}

// updateVacancyRequest — тело PATCH /api/vacancies/{id}.
// Каждое поле опционально: отсутствие означает «не трогать», null — «очистить».
type updateVacancyRequest struct {
	URL        Optional[string]     `json:"url"`
	Company    Optional[string]     `json:"company"`
	Grade      Optional[string]     `json:"grade"`
	TechTags   Optional[[]string]   `json:"tech_tags"`
	OpenedDate Optional[model.Date] `json:"opened_date"`
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
	if r.Company.Provided() && len(*r.Company.Value) > maxTextLen {
		errs.add("company", "слишком длинное значение")
	}
	if r.TechTags.Provided() {
		if msg := validateTags(*r.TechTags.Value); msg != "" {
			errs.add("tech_tags", msg)
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
