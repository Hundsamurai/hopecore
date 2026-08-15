package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Hundsamurai/hopecore/backend/internal/store"
)

// maxRequestBody ограничивает размер тела запроса. Инструмент локальный,
// но защита от случайно вставленного гигабайта текста не мешает.
const maxRequestBody = 1 << 20 // 1 МиБ

// listVacanciesResponse обёрнут в объект, а не отдан массивом: так позже
// можно добавить метаданные, не ломая клиента.
type listVacanciesResponse struct {
	Items []vacancyResponse `json:"items"`
}

func (s *Server) handleListVacancies(w http.ResponseWriter, r *http.Request) {
	filter := store.VacancyFilter{}
	errs := fieldErrors{}

	query := r.URL.Query()

	if raw := query.Get("include_inactive"); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			errs.add("include_inactive", "ожидается true или false")
		}
		filter.IncludeInactive = value
	}

	if raw := query.Get("sort"); raw != "" {
		if !store.IsSortableField(raw) {
			errs.add("sort", "ожидается одно из значений: "+strings.Join(store.SortableFields(), ", "))
		}
		filter.Sort = raw
	}

	// По умолчанию сортировка по убыванию: свежеизменённые вакансии сверху (п. 6 ТЗ).
	filter.Desc = true
	if raw := query.Get("order"); raw != "" {
		switch strings.ToLower(raw) {
		case "asc":
			filter.Desc = false
		case "desc":
			filter.Desc = true
		default:
			errs.add("order", "ожидается asc или desc")
		}
	}

	if !errs.empty() {
		writeValidationError(w, errs)
		return
	}

	vacancies, err := store.ListVacancies(s.db, filter)
	if err != nil {
		s.writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, listVacanciesResponse{Items: newVacancyListResponse(vacancies)})
}

func (s *Server) handleCreateVacancy(w http.ResponseWriter, r *http.Request) {
	var req createVacancyRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}

	if errs := req.validate(); !errs.empty() {
		writeValidationError(w, errs)
		return
	}

	vacancy := req.toModel()

	// Согласованность зарплатных полей проверяется на собранной записи:
	// по одному полю «от больше чем до» не увидеть.
	if errs := normalizeSalary(&vacancy); !errs.empty() {
		writeValidationError(w, errs)
		return
	}

	if err := store.CreateVacancy(s.db, &vacancy); err != nil {
		s.writeInternalError(w, r, err)
		return
	}

	s.log.Info("вакансия добавлена", "id", vacancy.ID, "url", vacancy.URL)
	writeJSON(w, http.StatusCreated, newVacancyResponse(vacancy))
}

func (s *Server) handleGetVacancy(w http.ResponseWriter, r *http.Request) {
	id, ok := s.vacancyID(w, r)
	if !ok {
		return
	}

	vacancy, err := store.GetVacancy(s.db, id)
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

func (s *Server) handleUpdateVacancy(w http.ResponseWriter, r *http.Request) {
	id, ok := s.vacancyID(w, r)
	if !ok {
		return
	}

	var req updateVacancyRequest
	if !s.decodeJSON(w, r, &req) {
		return
	}

	if errs := req.validate(); !errs.empty() {
		writeValidationError(w, errs)
		return
	}

	vacancy, err := store.GetVacancy(s.db, id)
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w, "вакансия не найдена")
		return
	}
	if err != nil {
		s.writeInternalError(w, r, err)
		return
	}

	req.apply(vacancy)

	// После PATCH одна граница вилки может прийти из тела, а вторая остаться
	// прежней — сравнивать нужно итоговые значения.
	if errs := normalizeSalary(vacancy); !errs.empty() {
		writeValidationError(w, errs)
		return
	}

	if err := store.SaveVacancy(s.db, vacancy); err != nil {
		s.writeInternalError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, newVacancyResponse(*vacancy))
}

func (s *Server) handleDeleteVacancy(w http.ResponseWriter, r *http.Request) {
	id, ok := s.vacancyID(w, r)
	if !ok {
		return
	}

	err := store.DeleteVacancy(s.db, id)
	if errors.Is(err, store.ErrNotFound) {
		writeNotFound(w, "вакансия не найдена")
		return
	}
	if err != nil {
		s.writeInternalError(w, r, err)
		return
	}

	s.log.Info("вакансия удалена", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// vacancyID разбирает {id} из пути и сам отвечает клиенту при ошибке.
func (s *Server) vacancyID(w http.ResponseWriter, r *http.Request) (uint, bool) {
	raw := r.PathValue("id")

	id, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || id == 0 {
		// Некорректный id — это не «сломанный запрос», а несуществующий адрес:
		// клиенту достаточно 404, как и для нормального id без записи.
		writeNotFound(w, "вакансия не найдена")
		return 0, false
	}
	return uint(id), true
}

// decodeJSON читает тело запроса, отвергая неизвестные поля: опечатка в имени
// поля должна быть заметна сразу, а не молча игнорироваться.
func (s *Server) decodeJSON(w http.ResponseWriter, r *http.Request, dest any) bool {
	body := http.MaxBytesReader(w, r.Body, maxRequestBody)

	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dest); err != nil {
		writeError(w, http.StatusBadRequest, CodeInvalidJSON, "не удалось разобрать тело запроса: "+err.Error())
		return false
	}

	// Второй Decode должен вернуть EOF: одно тело — один JSON-объект.
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, CodeInvalidJSON, "в теле запроса больше одного JSON-объекта")
		return false
	}
	return true
}
