// Package fetcher скачивает страницу вакансии и превращает её в текст,
// пригодный для отправки языковой модели.
//
// Пакет ничего не знает ни про БД, ни про провайдеров моделей: на входе ссылка,
// на выходе текст и диагностика. Headless-браузер сознательно не используется
// (см. docs/main/design-stage2.md, п. 3.4), поэтому страницы, которые рендерятся
// на JavaScript, дадут мало текста — это видно в Page.Chars и в предупреждениях.
package fetcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// UserAgent — как трекер представляется сайтам. Только ASCII: значения
// HTTP-заголовков за пределами ASCII стандартом не предусмотрены.
//
// У проверки активности (internal/activity) своя строка: подсистемы независимы
// и могут разойтись, если одной понадобятся особые заголовки.
const UserAgent = "hopecore-vacancy-tracker/0.1 (personal vacancy tracker)"

const (
	// maxRedirects — лимит цепочки перенаправлений.
	maxRedirects = 5
	// maxBodyBytes ограничивает скачиваемый объём: страница вакансии,
	// которая весит больше пяти мегабайт, — это уже не страница вакансии.
	maxBodyBytes = 5 << 20
	// minUsefulChars — ниже этого порога текст считается подозрительно коротким.
	// Обычная вакансия даёт тысячи символов; сотни означают каркас SPA
	// или страницу антибота.
	minUsefulChars = 500
)

// Page — результат скачивания и очистки.
type Page struct {
	// RequestURL — что просили скачать, FinalURL — где оказались после редиректов.
	RequestURL string
	FinalURL   string
	StatusCode int

	// Title — содержимое <title>, полезно само по себе: там обычно должность.
	Title string
	// Text — готовый текст для модели: заголовок, описание из meta и текст страницы.
	Text  string
	Chars int
	// Truncated сообщает, что текст обрезан по лимиту.
	Truncated bool
	// Warnings — то, о чём стоит предупредить пользователя, но что не мешает
	// попробовать извлечение.
	Warnings []string
}

// ErrorKind — вид неудачи. Вызывающий по нему решает, что записать в журнал
// и что показать пользователю.
type ErrorKind string

const (
	KindInvalidURL  ErrorKind = "invalid_url"
	KindNetwork     ErrorKind = "network"
	KindTimeout     ErrorKind = "timeout"
	KindStatus      ErrorKind = "http_status"
	KindContentType ErrorKind = "content_type"
)

// Error — неудача скачивания с понятным человеку сообщением.
type Error struct {
	Kind ErrorKind
	// StatusCode заполнен только для KindStatus; ноль означает, что ответа не было.
	StatusCode int
	Message    string
	err        error
}

func (e *Error) Error() string {
	return e.Message
}

func (e *Error) Unwrap() error {
	return e.err
}

// Fetcher скачивает страницы. Один экземпляр переиспользует транспорт,
// но каждому запросу выдаёт свежую банку cookie.
type Fetcher struct {
	transport http.RoundTripper
	timeout   time.Duration
	maxChars  int
}

// New собирает фетчер. maxChars ограничивает объём текста, уходящего в модель.
func New(timeout time.Duration, maxChars int) *Fetcher {
	return &Fetcher{
		transport: http.DefaultTransport,
		timeout:   timeout,
		maxChars:  maxChars,
	}
}

// Fetch скачивает страницу и возвращает очищенный текст.
func (f *Fetcher) Fetch(ctx context.Context, rawURL string) (Page, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return Page{}, &Error{Kind: KindInvalidURL, Message: "некорректная ссылка", err: err}
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return Page{}, &Error{Kind: KindInvalidURL, Message: "ожидается ссылка со схемой http или https"}
	}

	resp, err := f.do(ctx, parsed.String())
	if err != nil {
		return Page{}, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Page{}, &Error{
			Kind:       KindStatus,
			StatusCode: resp.StatusCode,
			Message:    fmt.Sprintf("сайт ответил %d, страницу прочитать не удалось", resp.StatusCode),
		}
	}

	if err := checkContentType(resp.Header.Get("Content-Type")); err != nil {
		return Page{}, err
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return Page{}, &Error{Kind: KindNetwork, Message: "не удалось дочитать страницу: " + err.Error(), err: err}
	}

	content := extract(string(body))

	page := Page{
		RequestURL: rawURL,
		FinalURL:   resp.Request.URL.String(),
		StatusCode: resp.StatusCode,
		Title:      content.title,
		Text:       content.text,
	}

	page.Text, page.Truncated = truncate(page.Text, f.maxChars)
	page.Chars = len([]rune(page.Text))

	page.Warnings = warnings(page, content)
	return page, nil
}

// do выполняет запрос со свежей банкой cookie.
//
// Cookie нужны не для слежки, а чтобы вообще получить страницу: часть сайтов
// (проверено на ozon.tech) сначала ставит cookie и редиректит обратно, и без
// их сохранения запрос уходит в бесконечный цикл перенаправлений.
// Банка своя на каждый запрос: cookie одного сайта не должны переживать
// до следующего скачивания.
func (f *Fetcher) do(ctx context.Context, target string) (*http.Response, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, &Error{Kind: KindNetwork, Message: "не удалось подготовить запрос", err: err}
	}

	client := &http.Client{
		Transport: f.transport,
		Jar:       jar,
		Timeout:   f.timeout,
		CheckRedirect: func(_ *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirects {
				return fmt.Errorf("больше %d перенаправлений", maxRedirects)
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, &Error{Kind: KindInvalidURL, Message: "некорректная ссылка", err: err}
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	req.Header.Set("Accept-Language", "ru,en;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, describeRequestError(err)
	}
	return resp, nil
}

func describeRequestError(err error) *Error {
	switch {
	case errors.Is(err, context.Canceled):
		return &Error{Kind: KindNetwork, Message: "скачивание прервано", err: err}
	case errors.Is(err, context.DeadlineExceeded):
		return &Error{Kind: KindTimeout, Message: "таймаут скачивания страницы", err: err}
	}

	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		if urlErr.Timeout() {
			return &Error{Kind: KindTimeout, Message: "таймаут скачивания страницы", err: err}
		}
		return &Error{Kind: KindNetwork, Message: "сайт недоступен: " + urlErr.Err.Error(), err: err}
	}
	return &Error{Kind: KindNetwork, Message: "сайт недоступен: " + err.Error(), err: err}
}

// checkContentType отсекает всё, что не является HTML: PDF и картинки
// отправлять в модель бессмысленно, а токены они сожгут.
func checkContentType(raw string) error {
	if raw == "" {
		// Без заголовка предполагаем HTML: отказ был бы строже, чем нужно.
		return nil
	}

	mediaType := strings.ToLower(strings.TrimSpace(strings.Split(raw, ";")[0]))
	switch mediaType {
	case "text/html", "application/xhtml+xml", "text/plain":
		return nil
	default:
		return &Error{
			Kind:    KindContentType,
			Message: fmt.Sprintf("страница отдана как %s, а нужен HTML", mediaType),
		}
	}
}

// truncate обрезает текст с конца: начало страницы содержит должность и условия,
// а низ — подвал сайта и похожие вакансии.
func truncate(text string, maxChars int) (string, bool) {
	if maxChars <= 0 {
		return text, false
	}

	runes := []rune(text)
	if len(runes) <= maxChars {
		return text, false
	}
	return strings.TrimSpace(string(runes[:maxChars])), true
}

func warnings(page Page, content content) []string {
	var result []string

	if page.Chars < minUsefulChars {
		result = append(result, fmt.Sprintf(
			"со страницы удалось прочитать только %d символов текста: возможно, она рендерится "+
				"на JavaScript или защищена от автоматических запросов. Модель может не найти данные",
			page.Chars))
	}
	if content.antibot {
		result = append(result, "похоже на страницу проверки браузера, а не на вакансию")
	}
	if page.Truncated {
		result = append(result, "текст страницы обрезан по лимиту: длинные описания могли не попасть в запрос")
	}
	return result
}
