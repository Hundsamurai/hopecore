package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/Hundsamurai/hopecore/backend/internal/store"
)

// bulkCheckTimeout ограничивает массовую проверку целиком.
//
// Согласовано с таймаутами вокруг: WriteTimeout сервера — 5 минут,
// proxy_read_timeout в nginx — 300 секунд. Запас нужен, потому что проверка
// ходит по внешним сайтам и упирается в их скорость, а не в нашу.
const bulkCheckTimeout = 4 * time.Minute

// setActivityRequest — тело PUT /api/vacancies/{id}/activity.
//
// Поле обязано присутствовать: null означает «снять override и вернуться
// к результату авто-проверки», а его отсутствие — что клиент прислал не то,
// что собирался.
type setActivityRequest struct {
	ManualIsActive Optional[bool] `json:"manual_is_active"`
}

func (s *Server) handleSetActivity(w http.ResponseWriter, r *http.Request) {
	id, ok := s.vacancyID(w, r)
	if !ok {
		return
	}

	var req setActivityRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}

	if !req.ManualIsActive.Set {
		errs := fieldErrors{}
		errs.add("manual_is_active", "обязательное поле: true, false или null для возврата к авто-проверке")
		writeValidationError(w, errs)
		return
	}

	vacancy, err := s.activity.SetManualActivity(r.Context(), id, req.ManualIsActive.Value)
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w, "вакансия не найдена")
		return
	}
	if err != nil {
		s.writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, newVacancyResponse(*vacancy))
}

func (s *Server) handleCheckVacancy(w http.ResponseWriter, r *http.Request) {
	id, ok := s.vacancyID(w, r)
	if !ok {
		return
	}

	vacancy, err := s.activity.CheckOne(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w, "вакансия не найдена")
		return
	}
	if err != nil {
		s.writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, newVacancyResponse(*vacancy))
}

func (s *Server) handleCheckAllVacancies(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), bulkCheckTimeout)
	defer cancel()

	summary, err := s.activity.CheckAll(ctx)
	if err != nil {
		// Клиент ушёл или сработал общий таймаут: часть результатов уже сохранена,
		// поэтому это не 500 — просто сообщаем, что прогон не доведён до конца.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			s.log.Warn("массовая проверка прервана", "checked", summary.Checked, "error", err)
			writeJSON(w, http.StatusOK, summary)
			return
		}
		s.writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, summary)
}
