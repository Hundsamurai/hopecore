// Package api содержит HTTP-слой: роутер, хендлеры и формат ответов.
package api

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/Hundsamurai/hopecore/backend/internal/service"
	"gorm.io/gorm"
)

// Server держит зависимости хендлеров.
type Server struct {
	log      *slog.Logger
	db       *gorm.DB
	activity *service.ActivityService
}

// NewServer создаёт HTTP-слой приложения.
func NewServer(log *slog.Logger, db *gorm.DB, activity *service.ActivityService) *Server {
	return &Server{log: log, db: db, activity: activity}
}

// Routes собирает роутер. Используется http.ServeMux из stdlib:
// с Go 1.22 он умеет паттерны с методом и wildcard-сегментами,
// поэтому внешний роутер не нужен.
func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", s.handleHealth)

	mux.HandleFunc("GET /api/vacancies", s.handleListVacancies)
	mux.HandleFunc("POST /api/vacancies", s.handleCreateVacancy)
	mux.HandleFunc("GET /api/vacancies/{id}", s.handleGetVacancy)
	mux.HandleFunc("PATCH /api/vacancies/{id}", s.handleUpdateVacancy)
	mux.HandleFunc("DELETE /api/vacancies/{id}", s.handleDeleteVacancy)

	// Проверка активности запускается только вручную, крона нет (п. 3 ТЗ).
	// Пути /vacancies/check и /vacancies/{id}/check не конфликтуют:
	// у них разное число сегментов.
	mux.HandleFunc("POST /api/vacancies/check", s.handleCheckAllVacancies)
	mux.HandleFunc("POST /api/vacancies/{id}/check", s.handleCheckVacancy)
	mux.HandleFunc("PUT /api/vacancies/{id}/activity", s.handleSetActivity)

	// Статус кандидата — 1:1 с вакансией, поэтому PUT с upsert-семантикой
	// вместо отдельных POST и PATCH.
	mux.HandleFunc("GET /api/vacancies/{id}/candidate-status", s.handleGetCandidateStatus)
	mux.HandleFunc("PUT /api/vacancies/{id}/candidate-status", s.handlePutCandidateStatus)

	// withJSONErrors переводит служебные 404/405 от ServeMux в общий JSON-формат.
	// Отдельный catch-all маршрут для этого не годится: он перехватывал бы путь
	// целиком и ServeMux перестал бы отдавать 405 на неверный метод.
	return s.withRequestLog(withJSONErrors(mux))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// writeJSON — единая точка сериализации успешных ответов.
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	// Ошибку кодирования логировать некому: заголовки уже отправлены,
	// поменять статус ответа нельзя. Клиент увидит обрыв тела.
	_ = json.NewEncoder(w).Encode(body)
}
