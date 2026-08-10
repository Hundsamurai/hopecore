package api

import (
	"net/http"
	"strings"
	"time"
)

// statusRecorder запоминает код ответа, чтобы его можно было залогировать.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// withRequestLog пишет по строке на запрос в stdout.
// Алертинга нет — по п. 7 ТЗ достаточно логов в консоль.
func (s *Server) withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		s.log.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration", time.Since(start).String(),
		)
	})
}

// jsonErrorWriter подменяет текстовые ответы ServeMux (404 «page not found»
// и 405 «Method Not Allowed») на JSON в общем формате.
//
// Ответы самих хендлеров не трогаются: они уже выставили Content-Type
// application/json, и это служит признаком «здесь всё под контролем».
type jsonErrorWriter struct {
	http.ResponseWriter
	replace bool
	status  int
}

func (w *jsonErrorWriter) WriteHeader(status int) {
	if (status == http.StatusNotFound || status == http.StatusMethodNotAllowed) &&
		!strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
		w.replace = true
		w.status = status
		return
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *jsonErrorWriter) Write(b []byte) (int, error) {
	if w.replace {
		// Проглатываем текстовое тело от ServeMux, отвечать будем сами.
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}

func withJSONErrors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrapped := &jsonErrorWriter{ResponseWriter: w}

		next.ServeHTTP(wrapped, r)

		if !wrapped.replace {
			return
		}

		switch wrapped.status {
		case http.StatusMethodNotAllowed:
			// Заголовок Allow ServeMux уже выставил, он уедет клиенту как есть.
			writeError(w, http.StatusMethodNotAllowed, CodeMethodNotAllowed,
				"метод "+r.Method+" не поддерживается для этого адреса")
		default:
			writeError(w, http.StatusNotFound, CodeNotFound, "метод API не найден")
		}
	})
}
