package llm

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

func TestExtractionSchemaMatchesVacancyFields(t *testing.T) {
	schema := ExtractionSchema()

	// Схема должна описывать ровно то, что есть куда сохранить:
	// извлекать поле, которому нет места в вакансии, значит жечь токены впустую.
	want := []string{
		"title", "company", "grade", "tech_tags", "opened_date",
		"salary_from", "salary_to", "salary_currency", "salary_gross", "work_format",
	}

	got := schema.FieldNames()
	if len(got) != len(want) {
		t.Fatalf("полей в схеме: %d, ожидалось %d (%v)", len(got), len(want), got)
	}
	for i, name := range want {
		if got[i] != name {
			t.Errorf("поле[%d] = %q, ожидалось %q", i, got[i], name)
		}
	}
}

func TestExtractionSchemaEnumsComeFromModel(t *testing.T) {
	schema := ExtractionSchema()

	enums := map[string][]string{}
	for _, field := range schema.Fields {
		if len(field.Enum) > 0 {
			enums[field.Name] = field.Enum
		}
	}

	// Наборы значений берутся из модели: расхождение означало бы, что модель
	// будет присылать значения, которые API отвергнет.
	for _, grade := range model.Grades {
		if !contains(enums["grade"], grade) {
			t.Errorf("грейд %q из модели отсутствует в схеме", grade)
		}
	}
	for _, format := range model.WorkFormats {
		if !contains(enums["work_format"], format) {
			t.Errorf("формат %q из модели отсутствует в схеме", format)
		}
	}

	// «Не нашёл» выражается через null, а не пустой строкой в наборе:
	// Gemini отвергает схему с пустым значением в enum.
	for _, name := range []string{"grade", "work_format"} {
		if contains(enums[name], "") {
			t.Errorf("в наборе %q есть пустая строка — Gemini такую схему отвергает", name)
		}
	}
	for _, field := range schema.Fields {
		if len(field.Enum) > 0 && !field.Nullable {
			t.Errorf("поле %q с набором значений не nullable — модели некуда деть «не нашёл»", field.Name)
		}
	}
}

func TestSchemaJSONSchema(t *testing.T) {
	schema := ExtractionSchema().JSONSchema()

	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("схема не сериализуется: %v", err)
	}

	if schema["type"] != "object" {
		t.Errorf("type = %v", schema["type"])
	}
	if schema["additionalProperties"] != false {
		t.Error("additionalProperties должно быть false: лишние поля модели не нужны")
	}

	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties не объект")
	}
	if len(properties) != 10 {
		t.Errorf("свойств: %d, ожидалось 10", len(properties))
	}

	required, ok := schema["required"].([]string)
	if !ok || len(required) != 10 {
		t.Errorf("required = %v: все поля должны быть обязательными, иначе пропуск не отличить от забывчивости", schema["required"])
	}

	// Nullable-поля должны допускать null, иначе модели придётся выдумывать.
	dateSchema := properties["opened_date"].(map[string]any)
	types, ok := dateSchema["type"].([]string)
	if !ok || !contains(types, "null") {
		t.Errorf("opened_date type = %v, ожидался список с null", dateSchema["type"])
	}

	// Массив тегов описан как массив строк.
	tagsSchema := properties["tech_tags"].(map[string]any)
	if tagsSchema["type"] != "array" {
		t.Errorf("tech_tags type = %v", tagsSchema["type"])
	}

	// У каждого поля есть описание: по нему модель понимает, что искать.
	for name, raw := range properties {
		field := raw.(map[string]any)
		if description, _ := field["description"].(string); strings.TrimSpace(description) == "" {
			t.Errorf("у поля %q нет описания", name)
		}
	}

	if !strings.Contains(string(raw), "salary_from") {
		t.Error("в сериализованной схеме нет salary_from")
	}
}

func TestSystemPromptStatesKeyRules(t *testing.T) {
	// Правила промпта — то, что защищает от выдумок. Если формулировки уедут,
	// это должно быть заметно.
	for _, rule := range []string{"Не выдумывай", "null", "вилка из объявления", "JSON"} {
		if !strings.Contains(SystemPrompt, rule) {
			t.Errorf("в системном промпте нет упоминания %q", rule)
		}
	}
}

func TestBuildUserPromptIncludesPageText(t *testing.T) {
	prompt := BuildUserPrompt("https://example.com/vacancy", "Текст вакансии про Go")

	for _, want := range []string{"https://example.com/vacancy", "Текст вакансии про Go"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("в промпте нет %q", want)
		}
	}

	// Без ссылки промпт тоже собирается.
	if strings.Contains(BuildUserPrompt("", "текст"), "Ссылка") {
		t.Error("упоминание ссылки при пустом URL")
	}
}

func TestPromptVersionSet(t *testing.T) {
	// Версия пишется в журнал: без неё нельзя объяснить старый результат.
	if strings.TrimSpace(PromptVersion) == "" {
		t.Fatal("PromptVersion пуст")
	}
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}
