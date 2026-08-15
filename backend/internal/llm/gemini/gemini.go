// Package gemini — адаптер Gemini через нативный структурированный вывод.
//
// Формат запроса и ответа сверен живыми вызовами, а не только документацией:
// схема передаётся в generationConfig.responseSchema, типы в ней пишутся
// заглавными, а «не нашёл» выражается через nullable, потому что пустую строку
// в enum API отвергает.
package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Hundsamurai/hopecore/backend/internal/llm"
)

const defaultBaseURL = "https://generativelanguage.googleapis.com/v1beta"

// maxResponseBytes ограничивает ответ провайдера: разумный ответ по нашей схеме
// весит единицы килобайт.
const maxResponseBytes = 1 << 20

// Provider реализует llm.Provider.
type Provider struct {
	apiKey string
	client *http.Client
	// BaseURL переопределяется в тестах, чтобы не ходить в сеть.
	BaseURL string
}

// New собирает адаптер.
func New(apiKey string, timeout time.Duration) *Provider {
	return &Provider{
		apiKey:  apiKey,
		client:  &http.Client{Timeout: timeout},
		BaseURL: defaultBaseURL,
	}
}

// ID возвращает идентификатор провайдера.
func (p *Provider) ID() string {
	return llm.ProviderGemini
}

// Complete отправляет запрос и возвращает JSON-ответ модели с расходом токенов.
func (p *Provider) Complete(ctx context.Context, req llm.Request) (llm.Response, error) {
	// Первая попытка — с отключёнными размышлениями: на извлечении данных они
	// не улучшают результат, но стоят втрое дороже самого ответа
	// (проверено: 117 токенов размышлений против 31 токена ответа).
	resp, err := p.call(ctx, req, true)
	if err == nil {
		return resp, nil
	}

	// Часть моделей (обычно pro-семейство) не позволяет отключать размышления.
	// Тогда повторяем без этой настройки, а не сдаёмся.
	if isThinkingRejected(err) {
		return p.call(ctx, req, false)
	}

	// Счётчики возвращаются вместе с ошибкой: обрезанный или отклонённый ответ
	// уже стоил токенов, и это должно попасть в журнал запусков.
	return resp, err
}

func (p *Provider) call(ctx context.Context, req llm.Request, disableThinking bool) (llm.Response, error) {
	body, err := json.Marshal(buildRequest(req, disableThinking))
	if err != nil {
		return llm.Response{}, fmt.Errorf("сборка запроса к Gemini: %w", err)
	}

	url := fmt.Sprintf("%s/models/%s:generateContent", strings.TrimSuffix(p.BaseURL, "/"), req.Model)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return llm.Response{}, fmt.Errorf("запрос к Gemini: %w", err)
	}
	// Ключ уходит заголовком, а не параметром строки запроса: так он не попадёт
	// в логи прокси и в историю обращений.
	httpReq.Header.Set("x-goog-api-key", p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("Gemini недоступен: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, httpResp.Body)
		_ = httpResp.Body.Close()
	}()

	raw, err := io.ReadAll(io.LimitReader(httpResp.Body, maxResponseBytes))
	if err != nil {
		return llm.Response{}, fmt.Errorf("чтение ответа Gemini: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return llm.Response{}, describeAPIError(httpResp.StatusCode, raw)
	}

	return parseResponse(raw)
}

// --- запрос ---

type request struct {
	SystemInstruction *instruction     `json:"systemInstruction,omitempty"`
	Contents          []content        `json:"contents"`
	GenerationConfig  generationConfig `json:"generationConfig"`
}

type instruction struct {
	Parts []part `json:"parts"`
}

type content struct {
	Role  string `json:"role"`
	Parts []part `json:"parts"`
}

type part struct {
	Text string `json:"text"`
}

type generationConfig struct {
	Temperature      float64        `json:"temperature"`
	ResponseMIMEType string         `json:"responseMimeType"`
	ResponseSchema   map[string]any `json:"responseSchema"`
	ThinkingConfig   *thinking      `json:"thinkingConfig,omitempty"`
}

type thinking struct {
	ThinkingBudget int `json:"thinkingBudget"`
}

func buildRequest(req llm.Request, disableThinking bool) request {
	out := request{
		Contents: []content{{Role: "user", Parts: []part{{Text: req.User}}}},
		GenerationConfig: generationConfig{
			// Извлечение данных — не творческая задача: нулевая температура
			// делает результат воспроизводимым.
			Temperature:      0,
			ResponseMIMEType: "application/json",
			ResponseSchema:   toGeminiSchema(req.Schema),
		},
	}

	if req.System != "" {
		out.SystemInstruction = &instruction{Parts: []part{{Text: req.System}}}
	}
	if disableThinking {
		out.GenerationConfig.ThinkingConfig = &thinking{ThinkingBudget: 0}
	}
	return out
}

// toGeminiSchema переводит внутреннюю схему в формат Gemini.
//
// Отличия от JSON Schema, выясненные на живых запросах:
//   - типы пишутся заглавными: OBJECT, STRING, NUMBER, BOOLEAN, ARRAY;
//   - null выражается полем nullable, а не списком типов;
//   - пустая строка в enum запрещена, поэтому она отбрасывается;
//   - additionalProperties не поддерживается и не отправляется.
func toGeminiSchema(schema llm.Schema) map[string]any {
	properties := make(map[string]any, len(schema.Fields))
	required := make([]string, 0, len(schema.Fields))
	order := make([]string, 0, len(schema.Fields))

	for _, field := range schema.Fields {
		properties[field.Name] = fieldSchema(field)
		required = append(required, field.Name)
		order = append(order, field.Name)
	}

	return map[string]any{
		"type":        "OBJECT",
		"properties":  properties,
		"required":    required,
		"description": schema.Description,
		// Порядок свойств делает ответы стабильнее между запросами.
		"propertyOrdering": order,
	}
}

func fieldSchema(field llm.Field) map[string]any {
	schema := map[string]any{"description": field.Description}

	switch field.Type {
	case llm.TypeStringArray:
		schema["type"] = "ARRAY"
		schema["items"] = map[string]any{"type": "STRING"}
	case llm.TypeNumber:
		schema["type"] = "NUMBER"
	case llm.TypeBoolean:
		schema["type"] = "BOOLEAN"
	default:
		schema["type"] = "STRING"
		if values := nonEmpty(field.Enum); len(values) > 0 {
			schema["enum"] = values
		}
	}

	if field.Nullable {
		schema["nullable"] = true
	}
	return schema
}

// nonEmpty убирает пустые значения: Gemini отвечает на них
// «enum[0]: cannot be empty».
func nonEmpty(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			result = append(result, value)
		}
	}
	return result
}

// --- ответ ---

type response struct {
	Candidates []struct {
		Content struct {
			Parts []part `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		// ThoughtsTokenCount — токены размышлений. Они тарифицируются как
		// выходные, поэтому попадают в OutputTokens: иначе оценка стоимости
		// врёт в разы.
		ThoughtsTokenCount int `json:"thoughtsTokenCount"`
	} `json:"usageMetadata"`
	PromptFeedback struct {
		BlockReason string `json:"blockReason"`
	} `json:"promptFeedback"`
}

func parseResponse(raw []byte) (llm.Response, error) {
	var parsed response
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return llm.Response{}, fmt.Errorf("ответ Gemini не разобран: %w", err)
	}

	usage := llm.Response{
		InputTokens:  parsed.UsageMetadata.PromptTokenCount,
		OutputTokens: parsed.UsageMetadata.CandidatesTokenCount + parsed.UsageMetadata.ThoughtsTokenCount,
	}

	if reason := parsed.PromptFeedback.BlockReason; reason != "" {
		return usage, fmt.Errorf("Gemini отклонил запрос: %s", reason)
	}
	if len(parsed.Candidates) == 0 {
		return usage, fmt.Errorf("Gemini не вернул ответ")
	}

	candidate := parsed.Candidates[0]
	switch candidate.FinishReason {
	case "", "STOP":
		// Обычное завершение.
	case "MAX_TOKENS":
		return usage, fmt.Errorf("ответ Gemini обрезан по лимиту токенов: страница слишком большая")
	default:
		return usage, fmt.Errorf("Gemini прервал ответ: %s", candidate.FinishReason)
	}

	var text strings.Builder
	for _, p := range candidate.Content.Parts {
		text.WriteString(p.Text)
	}
	if strings.TrimSpace(text.String()) == "" {
		return usage, fmt.Errorf("Gemini вернул пустой ответ")
	}

	usage.JSON = []byte(text.String())
	return usage, nil
}

type apiError struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// describeAPIError переводит ошибку провайдера в понятное сообщение.
func describeAPIError(statusCode int, raw []byte) error {
	var parsed apiError
	message := strings.TrimSpace(string(raw))
	if err := json.Unmarshal(raw, &parsed); err == nil && parsed.Error.Message != "" {
		message = parsed.Error.Message
	}
	if len(message) > 500 {
		message = message[:500]
	}

	switch statusCode {
	case http.StatusTooManyRequests:
		return fmt.Errorf("Gemini: превышена квота или частота запросов — %s", message)
	case http.StatusUnauthorized, http.StatusForbidden:
		return fmt.Errorf("Gemini отклонил ключ (%d): %s", statusCode, message)
	case http.StatusNotFound:
		return fmt.Errorf("Gemini: модель недоступна — %s", message)
	default:
		return fmt.Errorf("Gemini ответил %d: %s", statusCode, message)
	}
}

// isThinkingRejected распознаёт отказ модели отключать размышления.
func isThinkingRejected(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "thinking") || strings.Contains(message, "thinking_config")
}
