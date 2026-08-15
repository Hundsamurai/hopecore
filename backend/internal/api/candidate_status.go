package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
	"github.com/Hundsamurai/hopecore/backend/internal/store"
)

// putCandidateStatusRequest — тело PUT /api/vacancies/{id}/candidate-status.
//
// PUT заменяет представление целиком: отсутствующее поле означает пустое значение,
// а не «не трогать». Так и работает форма в UI — она присылает всё своё состояние.
// Частичные правки здесь не нужны: форма одна и небольшая.
type putCandidateStatusRequest struct {
	CoverLetter        string      `json:"cover_letter"`
	SentAt             *model.Date `json:"sent_at"`
	InterviewStage     string      `json:"interview_stage"`
	HRContact          string      `json:"hr_contact"`
	InterviewRecordURL string      `json:"interview_record_url"`
	OfferReceived      bool        `json:"offer_received"`
	OfferedSalary      *float64    `json:"offered_salary"`
	RealSalary         *float64    `json:"real_salary"`
	MarketSalaryData   string      `json:"market_salary_data"`
}

func (r putCandidateStatusRequest) validate() fieldErrors {
	errs := fieldErrors{}

	// Сопроводительное письмо длиннее остальных полей: это связный текст.
	if len(r.CoverLetter) > 20000 {
		errs.add("cover_letter", "слишком длинный текст")
	}
	for field, value := range map[string]string{
		"interview_stage":    r.InterviewStage,
		"hr_contact":         r.HRContact,
		"market_salary_data": r.MarketSalaryData,
	} {
		if len(value) > maxTextLen {
			errs.add(field, "слишком длинное значение")
		}
	}

	// Ссылка на запись собеседования опциональна, но если есть — должна быть ссылкой.
	if strings.TrimSpace(r.InterviewRecordURL) != "" {
		if msg := validateURL(r.InterviewRecordURL); msg != "" {
			errs.add("interview_record_url", msg)
		}
	}

	if msg := validateSalaryBound(r.OfferedSalary); msg != "" {
		errs.add("offered_salary", msg)
	}
	if msg := validateSalaryBound(r.RealSalary); msg != "" {
		errs.add("real_salary", msg)
	}

	return errs
}

func (r putCandidateStatusRequest) toModel(vacancyID uint) model.CandidateStatus {
	return model.CandidateStatus{
		VacancyID:          vacancyID,
		CoverLetter:        strings.TrimSpace(r.CoverLetter),
		SentAt:             r.SentAt,
		InterviewStage:     strings.TrimSpace(r.InterviewStage),
		HRContact:          strings.TrimSpace(r.HRContact),
		InterviewRecordURL: strings.TrimSpace(r.InterviewRecordURL),
		OfferReceived:      r.OfferReceived,
		OfferedSalary:      r.OfferedSalary,
		RealSalary:         r.RealSalary,
		MarketSalaryData:   strings.TrimSpace(r.MarketSalaryData),
	}
}

func (s *Server) handlePutCandidateStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := s.vacancyID(w, r)
	if !ok {
		return
	}

	var req putCandidateStatusRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}

	if errs := req.validate(); !errs.empty() {
		writeValidationError(w, errs)
		return
	}

	status := req.toModel(id)
	err := store.UpsertCandidateStatus(s.db, &status)
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w, "вакансия не найдена")
		return
	}
	if err != nil {
		s.writeInternalError(w, r, err)
		return
	}

	s.log.Info("статус кандидата сохранён", "vacancy_id", id, "stage", status.InterviewStage)
	writeJSON(w, http.StatusOK, newCandidateStatusResponse(status))
}

func (s *Server) handleGetCandidateStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := s.vacancyID(w, r)
	if !ok {
		return
	}

	status, err := store.GetCandidateStatus(s.db, id)
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w, "статус кандидата по этой вакансии не заполнен")
		return
	}
	if err != nil {
		s.writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, newCandidateStatusResponse(*status))
}
