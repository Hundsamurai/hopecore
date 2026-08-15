package llm

import "context"

// FieldType — тип поля в схеме ответа. Набор намеренно узкий: чем меньше
// конструкций, тем проще перевести схему в формат каждого провайдера.
type FieldType string

const (
	TypeString      FieldType = "string"
	TypeNumber      FieldType = "number"
	TypeBoolean     FieldType = "boolean"
	TypeStringArray FieldType = "string_array"
)

// Field — одно поле ожидаемого ответа.
type Field struct {
	Name        string
	Type        FieldType
	Description string
	// Enum ограничивает значения. Пустая строка в списке означает
	// «модель не нашла значение» и разрешена намеренно: она честнее выдумки.
	Enum []string
	// Nullable разрешает null. Для чисел и дат это единственный способ
	// сказать «на странице этого нет».
	Nullable bool
}

// Schema — описание ожидаемого ответа во внутреннем виде.
//
// Провайдеры принимают схему по-разному: Gemini — responseSchema, Claude —
// JSON Schema или определение tool, DeepSeek — только режим валидного JSON,
// и схему ему приходится описывать словами в промпте. Поэтому схема одна,
// а перевод в конкретный формат живёт в адаптерах.
type Schema struct {
	Name        string
	Description string
	Fields      []Field
}

// JSONSchema переводит схему в JSON Schema — форму, которую понимают Claude
// и, после сериализации, годится для описания в промпте DeepSeek.
func (s Schema) JSONSchema() map[string]any {
	properties := make(map[string]any, len(s.Fields))
	required := make([]string, 0, len(s.Fields))

	for _, field := range s.Fields {
		properties[field.Name] = field.jsonSchema()
		// Все поля обязательны: пустая строка или null — это тоже ответ,
		// а вот пропущенное поле не отличить от забытого.
		required = append(required, field.Name)
	}

	return map[string]any{
		"type":                 "object",
		"description":          s.Description,
		"properties":           properties,
		"required":             required,
		"additionalProperties": false,
	}
}

func (f Field) jsonSchema() map[string]any {
	schema := map[string]any{"description": f.Description}

	switch f.Type {
	case TypeStringArray:
		schema["type"] = "array"
		schema["items"] = map[string]any{"type": "string"}
	case TypeNumber:
		schema["type"] = f.nullableType("number")
	case TypeBoolean:
		schema["type"] = f.nullableType("boolean")
	default:
		schema["type"] = f.nullableType("string")
		if len(f.Enum) > 0 {
			values := make([]any, 0, len(f.Enum)+1)
			for _, value := range f.Enum {
				values = append(values, value)
			}
			// В JSON Schema null нужно разрешить и в enum, иначе строгий
			// валидатор отвергнет законное «не нашёл».
			if f.Nullable {
				values = append(values, nil)
			}
			schema["enum"] = values
		}
	}

	return schema
}

// nullableType отдаёт тип в виде списка, если разрешён null:
// так это выражается в JSON Schema.
func (f Field) nullableType(base string) any {
	if f.Nullable {
		return []string{base, "null"}
	}
	return base
}

// FieldNames перечисляет поля схемы в порядке объявления.
func (s Schema) FieldNames() []string {
	names := make([]string, 0, len(s.Fields))
	for _, field := range s.Fields {
		names = append(names, field.Name)
	}
	return names
}

// Request — запрос к модели на извлечение структурированных данных.
type Request struct {
	Model  string
	System string
	User   string
	Schema Schema
}

// Response — ответ модели вместе с расходом токенов.
type Response struct {
	// JSON — тело ответа модели. Разбирать и валидировать его будет вызывающий:
	// адаптер отвечает только за транспорт.
	JSON []byte
	// InputTokens и OutputTokens приходят от провайдера; ноль означает,
	// что провайдер счётчиков не вернул.
	InputTokens  int
	OutputTokens int
}

// Provider — адаптер конкретного провайдера. Интерфейс существует ради тестов:
// в них подставляется предсказуемая реализация вместо реальной сети.
type Provider interface {
	ID() string
	Complete(ctx context.Context, req Request) (Response, error)
}
