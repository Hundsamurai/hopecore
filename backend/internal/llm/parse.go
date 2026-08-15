package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

// maxTagLen и maxTags повторяют ограничения API: извлечённое значение должно
// проходить те же проверки, что и введённое руками.
const (
	maxTextLen = 2000
	maxTagLen  = 100
	maxTags    = 50
)

// ErrNoJSON возвращается, когда в ответе нет разбираемого JSON.
// Это единственная причина повторить запрос к модели.
var ErrNoJSON = errors.New("в ответе модели нет корректного JSON")

// Values — извлечённые значения. Пустая строка, пустой массив и nil означают
// «модель не нашла», а не «нужно очистить поле».
type Values struct {
	Title          string
	Company        string
	Grade          string
	TechTags       []string
	OpenedDate     *model.Date
	SalaryFrom     *float64
	SalaryTo       *float64
	SalaryCurrency string
	SalaryGross    *bool
	WorkFormat     string
}

// FieldNote — замечание по одному полю.
type FieldNote struct {
	Field string `json:"field"`
	Note  string `json:"note"`
}

// Result — итог разбора ответа модели.
type Result struct {
	Values Values
	// Filled — поля, которые модель заполнила и которые прошли проверку.
	// Только они предлагаются к применению: остальные либо пусты, либо отброшены.
	Filled map[string]bool
	// Notes — что отбросили и почему. Одно негодное поле не обнуляет остальные:
	// выдуманная дата не должна стоить пятнадцати корректных значений.
	Notes []FieldNote
}

// IsEmpty сообщает, что модель не нашла ничего пригодного.
func (r Result) IsEmpty() bool {
	return len(r.Filled) == 0
}

// placeholders — то, чем модели заполняют поля вместо честного «нет данных».
// Такие значения приравниваются к пустым молча: смысл у них тот же.
var placeholders = map[string]bool{
	"n/a": true, "na": true, "none": true, "null": true, "nil": true,
	"unknown": true, "not specified": true, "not mentioned": true,
	"не указано": true, "не указана": true, "не указан": true,
	"неизвестно": true, "нет данных": true, "отсутствует": true,
	"-": true, "—": true, "–": true, "?": true, "": true,
}

var currencyPattern = regexp.MustCompile(`^[A-Za-z]{3}$`)

// Parse разбирает ответ модели и проверяет значения.
//
// Ошибка возвращается только если JSON вообще не разобрался: всё остальное —
// замечания по полям. Это осознанно: ответ модели почти всегда полезен
// хотя бы частично.
func Parse(raw []byte) (Result, error) {
	payload, err := extractJSONObject(raw)
	if err != nil {
		return Result{}, err
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrNoJSON, err)
	}

	result := Result{Filled: map[string]bool{}}

	known := map[string]bool{}
	for _, name := range ExtractionSchema().FieldNames() {
		known[name] = true
	}
	for name := range fields {
		if !known[name] {
			result.note(name, "модель вернула поле, которого нет в схеме — значение проигнорировано")
		}
	}

	result.parseText(fields, "title", &result.Values.Title)
	result.parseText(fields, "company", &result.Values.Company)
	result.parseEnum(fields, "grade", model.Grades, &result.Values.Grade)
	result.parseEnum(fields, "work_format", model.WorkFormats, &result.Values.WorkFormat)
	result.parseTags(fields)
	result.parseDate(fields)
	result.parseSalary(fields)

	return result, nil
}

func (r *Result) note(field, note string) {
	r.Notes = append(r.Notes, FieldNote{Field: field, Note: note})
}

func (r *Result) fill(field string) {
	r.Filled[field] = true
}

// rawString достаёт строковое значение, терпимо относясь к типам:
// модель может прислать число там, где по схеме строка.
func rawString(raw json.RawMessage) (string, bool) {
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString), true
	}

	var asNumber float64
	if err := json.Unmarshal(raw, &asNumber); err == nil {
		return strconv.FormatFloat(asNumber, 'f', -1, 64), true
	}
	return "", false
}

func isPlaceholder(value string) bool {
	return placeholders[strings.ToLower(strings.TrimSpace(value))]
}

func (r *Result) parseText(fields map[string]json.RawMessage, name string, dest *string) {
	raw, ok := fields[name]
	if !ok {
		return
	}

	value, ok := rawString(raw)
	if !ok {
		r.note(name, "ожидалась строка")
		return
	}
	if isPlaceholder(value) {
		return
	}

	// Переводы строк в должности и названии компании — почти всегда следствие
	// склейки соседних элементов разметки.
	value = strings.Join(strings.Fields(value), " ")

	if len([]rune(value)) > maxTextLen {
		value = string([]rune(value)[:maxTextLen])
		r.note(name, "значение слишком длинное и обрезано")
	}

	*dest = value
	r.fill(name)
}

func (r *Result) parseEnum(fields map[string]json.RawMessage, name string, allowed []string, dest *string) {
	raw, ok := fields[name]
	if !ok {
		return
	}

	value, ok := rawString(raw)
	if !ok {
		r.note(name, "ожидалась строка")
		return
	}
	if isPlaceholder(value) {
		return
	}

	// Регистр приводим сами: модели любят «Middle» и «REMOTE».
	normalized := strings.ToLower(value)
	for _, candidate := range allowed {
		if candidate == normalized {
			*dest = candidate
			r.fill(name)
			return
		}
	}

	r.note(name, fmt.Sprintf("значение %q не входит в набор (%s) — отброшено",
		value, strings.Join(allowed, ", ")))
}

func (r *Result) parseTags(fields map[string]json.RawMessage) {
	raw, ok := fields["tech_tags"]
	if !ok {
		return
	}

	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		// Иногда модель присылает строку «go, postgres» вместо массива.
		if value, ok := rawString(raw); ok && value != "" {
			list = strings.Split(value, ",")
			r.note("tech_tags", "модель вернула строку вместо массива — разобрано по запятым")
		} else {
			r.note("tech_tags", "ожидался массив строк")
			return
		}
	}

	seen := map[string]bool{}
	tags := make([]string, 0, len(list))
	dropped := 0

	for _, item := range list {
		tag := strings.Join(strings.Fields(item), " ")
		if tag == "" || isPlaceholder(tag) {
			continue
		}
		if len([]rune(tag)) > maxTagLen {
			dropped++
			continue
		}
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		tags = append(tags, tag)
	}

	if dropped > 0 {
		r.note("tech_tags", fmt.Sprintf("отброшено слишком длинных тегов: %d", dropped))
	}
	if len(tags) > maxTags {
		tags = tags[:maxTags]
		r.note("tech_tags", fmt.Sprintf("оставлены первые %d тегов", maxTags))
	}
	if len(tags) == 0 {
		return
	}

	r.Values.TechTags = tags
	r.fill("tech_tags")
}

func (r *Result) parseDate(fields map[string]json.RawMessage) {
	raw, ok := fields["opened_date"]
	if !ok {
		return
	}
	if isNull(raw) {
		return
	}

	value, ok := rawString(raw)
	if !ok {
		r.note("opened_date", "ожидалась строка с датой")
		return
	}
	if isPlaceholder(value) {
		return
	}

	parsed, err := model.ParseDate(value)
	if err != nil {
		r.note("opened_date", fmt.Sprintf("дата %q не разобрана, ожидается формат YYYY-MM-DD — отброшена", value))
		return
	}

	// Защита от выдуманных дат: вакансия из 1970 года или из будущего через
	// пять лет — признак того, что модель что-то сочинила.
	year := parsed.Year()
	upper := time.Now().UTC().Year() + 1
	if year < 2000 || year > upper {
		r.note("opened_date", fmt.Sprintf("дата %s выглядит неправдоподобно — отброшена", parsed))
		return
	}

	r.Values.OpenedDate = &parsed
	r.fill("opened_date")
}

func (r *Result) parseSalary(fields map[string]json.RawMessage) {
	from, fromOK := r.parseSalaryBound(fields, "salary_from")
	to, toOK := r.parseSalaryBound(fields, "salary_to")

	// Если границы противоречат друг другу, непонятно, какая из них выдумана,
	// поэтому отбрасываются обе.
	if fromOK && toOK && *from > *to {
		r.note("salary_from", "нижняя граница больше верхней — вилка отброшена целиком")
		return
	}

	if fromOK {
		r.Values.SalaryFrom = from
		r.fill("salary_from")
	}
	if toOK {
		r.Values.SalaryTo = to
		r.fill("salary_to")
	}

	hasSalary := fromOK || toOK
	r.parseCurrency(fields, hasSalary)
	r.parseGross(fields, hasSalary)
}

func (r *Result) parseSalaryBound(fields map[string]json.RawMessage, name string) (*float64, bool) {
	raw, ok := fields[name]
	if !ok || isNull(raw) {
		return nil, false
	}

	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		// «300 000 ₽» вместо числа — частый случай.
		text, textOK := rawString(raw)
		if !textOK {
			r.note(name, "ожидалось число")
			return nil, false
		}
		if isPlaceholder(text) {
			return nil, false
		}
		parsed, err := parseLooseNumber(text)
		if err != nil {
			r.note(name, fmt.Sprintf("значение %q не похоже на число — отброшено", text))
			return nil, false
		}
		r.note(name, fmt.Sprintf("значение %q приведено к числу %.0f", text, parsed))
		value = parsed
	}

	switch {
	case math.IsNaN(value) || math.IsInf(value, 0):
		r.note(name, "значение не является числом — отброшено")
		return nil, false
	case value < 0:
		r.note(name, "зарплата отрицательная — отброшена")
		return nil, false
	case value > model.MaxSalary:
		r.note(name, "неправдоподобно большая зарплата — отброшена")
		return nil, false
	case value == 0:
		// Ноль — это не вилка, а «не указано».
		return nil, false
	}

	return &value, true
}

func (r *Result) parseCurrency(fields map[string]json.RawMessage, hasSalary bool) {
	raw, ok := fields["salary_currency"]
	if !ok || isNull(raw) {
		return
	}

	value, ok := rawString(raw)
	if !ok {
		r.note("salary_currency", "ожидалась строка")
		return
	}
	if isPlaceholder(value) {
		return
	}

	// Символы валют модель присылает чаще, чем коды.
	if code, found := currencyBySymbol(value); found {
		value = code
	}
	if !currencyPattern.MatchString(value) {
		r.note("salary_currency", fmt.Sprintf("код валюты %q не распознан — отброшен", value))
		return
	}
	if !hasSalary {
		// Валюта без вилки ничего не означает, как и в API.
		return
	}

	r.Values.SalaryCurrency = strings.ToUpper(value)
	r.fill("salary_currency")
}

func (r *Result) parseGross(fields map[string]json.RawMessage, hasSalary bool) {
	raw, ok := fields["salary_gross"]
	if !ok || isNull(raw) || !hasSalary {
		return
	}

	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		text, textOK := rawString(raw)
		if !textOK || isPlaceholder(text) {
			return
		}
		switch strings.ToLower(text) {
		case "gross", "до вычета", "true":
			value = true
		case "net", "на руки", "false":
			value = false
		default:
			r.note("salary_gross", fmt.Sprintf("значение %q не распознано — отброшено", text))
			return
		}
	}

	r.Values.SalaryGross = &value
	r.fill("salary_gross")
}

func isNull(raw json.RawMessage) bool {
	return strings.TrimSpace(string(raw)) == "null"
}

var currencySymbols = map[string]string{
	"₽": "RUB", "руб": "RUB", "руб.": "RUB", "р.": "RUB", "рублей": "RUB",
	"$": "USD", "€": "EUR", "£": "GBP", "₸": "KZT", "₾": "GEL", "₴": "UAH",
}

func currencyBySymbol(value string) (string, bool) {
	code, ok := currencySymbols[strings.ToLower(strings.TrimSpace(value))]
	return code, ok
}

// parseLooseNumber разбирает «300 000», «300000 ₽», «300 тыс», «300k».
func parseLooseNumber(text string) (float64, error) {
	lower := strings.ToLower(text)

	multiplier := 1.0
	for _, suffix := range []struct {
		token string
		mult  float64
	}{
		{"тыс", 1000}, {"тысяч", 1000}, {"k", 1000}, {"к", 1000},
		{"млн", 1_000_000}, {"m", 1_000_000},
	} {
		if strings.Contains(lower, suffix.token) {
			multiplier = suffix.mult
			break
		}
	}

	var digits strings.Builder
	for _, r := range lower {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case r == ',' || r == '.':
			// Разделитель дробной части нам не нужен: зарплаты целые,
			// а запятая чаще разделяет разряды.
			continue
		}
	}

	if digits.Len() == 0 {
		return 0, fmt.Errorf("в значении %q нет цифр", text)
	}

	value, err := strconv.ParseFloat(digits.String(), 64)
	if err != nil {
		return 0, err
	}
	return value * multiplier, nil
}

// extractJSONObject достаёт JSON-объект из ответа модели.
//
// Даже в структурированном режиме ответ иногда приходит обёрнутым в ```json,
// а у DeepSeek строгой схемы нет вовсе. Поэтому берём подстроку от первой
// фигурной скобки до последней.
func extractJSONObject(raw []byte) ([]byte, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil, fmt.Errorf("%w: ответ пуст", ErrNoJSON)
	}

	if fenced := strings.Index(text, "```"); fenced >= 0 {
		text = stripCodeFence(text)
	}

	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return nil, fmt.Errorf("%w: объект не найден", ErrNoJSON)
	}

	return []byte(text[start : end+1]), nil
}

func stripCodeFence(text string) string {
	lines := strings.Split(text, "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
