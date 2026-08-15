package activity

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

// UserAgent — как трекер представляется сайтам вакансий. Честная строка вместо
// маскировки под браузер: инструмент личный, скрывать нечего.
//
// Только ASCII: значения HTTP-заголовков за пределами ASCII не предусмотрены
// стандартом, и часть клиентов и серверов на кириллице ломается.
const UserAgent = "hopecore-vacancy-tracker/0.1 (personal vacancy tracker)"

// maxRedirects ограничивает цепочку перенаправлений.
const maxRedirects = 5

// drainLimit — сколько байт тела вычитывается перед закрытием соединения.
// Тело нам не нужно (эвристики по тексту отложены, п. 6.1 дизайна), но частичное
// чтение позволяет переиспользовать keep-alive соединение.
const drainLimit = 32 << 10

// Result — итог одной проверки ссылки.
type Result struct {
	// Active: true — вакансия активна, false — снята, nil — определить не удалось.
	// nil никогда не затирает предыдущее значение в БД.
	Active *bool
	// StatusCode — код ответа; nil, если ответа не было вовсе.
	StatusCode *int
	// Err — человекочитаемая причина, по которой результат неизвестен.
	Err string
}

// Unknown сообщает, что проверка не дала определённого ответа.
func (r Result) Unknown() bool {
	return r.Active == nil
}

// Checker опрашивает ссылку на вакансию. Интерфейс нужен, чтобы в тестах
// подставлять предсказуемую реализацию вместо реальной сети.
type Checker interface {
	Check(ctx context.Context, rawURL string) Result
}

// Classify переводит код ответа в вывод об активности вакансии
// (таблица в п. 6.1 дизайн-документа).
//
//	2xx        — вакансия на месте;
//	404, 410   — страницы нет, считаем снятой;
//	остальное  — вывод не делаем: 429 и 5xx говорят о состоянии сервера,
//	             а 401/403 о защите от роботов, но не о судьбе вакансии.
func Classify(statusCode int) *bool {
	switch {
	case statusCode >= 200 && statusCode < 300:
		active := true
		return &active
	case statusCode == http.StatusNotFound || statusCode == http.StatusGone:
		active := false
		return &active
	default:
		return nil
	}
}

// HTTPChecker — реализация Checker поверх net/http.
type HTTPChecker struct {
	client *http.Client
}

// NewHTTPChecker собирает чекер с заданным таймаутом на запрос.
func NewHTTPChecker(timeout time.Duration) *HTTPChecker {
	return &HTTPChecker{
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= maxRedirects {
					return fmt.Errorf("больше %d перенаправлений", maxRedirects)
				}
				return nil
			},
		},
	}
}

// Check выполняет GET по ссылке и классифицирует ответ.
//
// Метод GET, а не HEAD: часть сайтов отвечает на HEAD 405 или отдаёт другой код,
// и проверка стала бы бесполезной.
func (c *HTTPChecker) Check(ctx context.Context, rawURL string) Result {
	if _, err := url.Parse(rawURL); err != nil {
		return Result{Err: "некорректная ссылка"}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return Result{Err: "некорректная ссылка"}
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := c.client.Do(req)
	if err != nil {
		return Result{Err: describeRequestError(err)}
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, drainLimit))
		_ = resp.Body.Close()
	}()

	statusCode := resp.StatusCode
	result := Result{
		Active:     Classify(statusCode),
		StatusCode: &statusCode,
	}
	if result.Unknown() {
		result.Err = fmt.Sprintf("код ответа %d не позволяет судить об активности", statusCode)
	}
	return result
}

// describeRequestError переводит ошибку транспорта в короткое понятное сообщение:
// клиенту в UI нужен смысл, а не полный текст с адресами и портами.
func describeRequestError(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "проверка прервана"
	case errors.Is(err, context.DeadlineExceeded):
		return "таймаут запроса"
	default:
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			if urlErr.Timeout() {
				return "таймаут запроса"
			}
			return "сайт недоступен: " + urlErr.Err.Error()
		}
		return "сайт недоступен: " + err.Error()
	}
}

// ApplyResult переносит итог проверки в вакансию.
//
// Ключевое правило этапа: неопределённый результат (Active == nil) не затирает
// прежнее значение auto_is_active, а ручной override не трогается вообще —
// решение пользователя авто-проверка не отменяет.
func ApplyResult(v *model.Vacancy, res Result, now time.Time) {
	v.LastCheckedAt = &now
	v.LastCheckCode = res.StatusCode
	v.LastCheckError = res.Err

	if res.Active != nil {
		v.AutoIsActive = res.Active
	}
}
