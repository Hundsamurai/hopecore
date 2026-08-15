package api

import (
	"net/http"
)

// Коды ошибок API. Клиент опирается на code, а message показывает пользователю.
const (
	CodeValidationFailed = "validation_failed"
	CodeInvalidJSON      = "invalid_json"
	CodeNotFound         = "not_found"
	CodeMethodNotAllowed = "method_not_allowed"
	CodeInternal         = "internal_error"
	// CodeProviderUnavailable — провайдер не настроен или не выбран.
	CodeProviderUnavailable = "provider_unavailable"
	// CodeExtractionFailed — страницу не удалось прочитать либо модель
	// не дала пригодного ответа. Подробности в журнале запусков.
	CodeExtractionFailed = "extraction_failed"
)

// errorBody — единый формат ошибок (docs/main/design-stage1.md, п. 7).
type errorBody struct {
	Error errorPayload `json:"error"`
}

type errorPayload struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// fieldErrors накапливает ошибки валидации по полям запроса,
// чтобы форма на фронте показала их разом, а не по одной.
type fieldErrors map[string]string

func (f fieldErrors) add(field, message string) {
	if _, exists := f[field]; !exists {
		f[field] = message
	}
}

func (f fieldErrors) empty() bool {
	return len(f) == 0
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorBody{Error: errorPayload{Code: code, Message: message}})
}

func writeValidationError(w http.ResponseWriter, fields fieldErrors) {
	writeJSON(w, http.StatusBadRequest, errorBody{Error: errorPayload{
		Code:    CodeValidationFailed,
		Message: "некорректные данные запроса",
		Fields:  fields,
	}})
}

func writeNotFound(w http.ResponseWriter, message string) {
	writeError(w, http.StatusNotFound, CodeNotFound, message)
}

// writeInternalError отдаёт клиенту обезличенный ответ, а подробности пишет в лог:
// текст ошибки БД в UI бесполезен и только мешает.
func (s *Server) writeInternalError(w http.ResponseWriter, r *http.Request, err error) {
	s.log.Error("внутренняя ошибка",
		"method", r.Method,
		"path", r.URL.Path,
		"error", err,
	)
	writeError(w, http.StatusInternalServerError, CodeInternal, "внутренняя ошибка сервера")
}
