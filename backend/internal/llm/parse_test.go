package llm

import (
	"errors"
	"strings"
	"testing"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

// goodAnswer — то, что должна возвращать модель на нормальной странице.
const goodAnswer = `{
  "title": "Go-разработчик",
  "company": "ООО Пример",
  "grade": "senior",
  "tech_tags": ["Go", "PostgreSQL", "Kubernetes"],
  "opened_date": "2026-08-01",
  "salary_from": 300000,
  "salary_to": 450000,
  "salary_currency": "RUB",
  "salary_gross": true,
  "work_format": "remote"
}`

func TestParseGoodAnswer(t *testing.T) {
	result, err := Parse([]byte(goodAnswer))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(result.Notes) != 0 {
		t.Errorf("замечания на корректном ответе: %+v", result.Notes)
	}

	v := result.Values
	if v.Title != "Go-разработчик" || v.Company != "ООО Пример" {
		t.Errorf("текстовые поля: %+v", v)
	}
	if v.Grade != model.GradeSenior || v.WorkFormat != model.WorkFormatRemote {
		t.Errorf("наборы: grade=%q work_format=%q", v.Grade, v.WorkFormat)
	}
	if len(v.TechTags) != 3 || v.TechTags[0] != "Go" {
		t.Errorf("tech_tags = %v", v.TechTags)
	}
	if v.OpenedDate == nil || v.OpenedDate.String() != "2026-08-01" {
		t.Errorf("opened_date = %v", v.OpenedDate)
	}
	if v.SalaryFrom == nil || *v.SalaryFrom != 300000 || v.SalaryTo == nil || *v.SalaryTo != 450000 {
		t.Errorf("вилка = %v / %v", v.SalaryFrom, v.SalaryTo)
	}
	if v.SalaryCurrency != "RUB" {
		t.Errorf("salary_currency = %q", v.SalaryCurrency)
	}
	if v.SalaryGross == nil || !*v.SalaryGross {
		t.Errorf("salary_gross = %v", v.SalaryGross)
	}

	// Все десять полей заполнены и предлагаются к применению.
	for _, name := range ExtractionSchema().FieldNames() {
		if !result.Filled[name] {
			t.Errorf("поле %q не отмечено как заполненное", name)
		}
	}
}

func TestParseEmptyAnswerIsValid(t *testing.T) {
	// Модель ничего не нашла — это законный ответ, а не ошибка.
	empty := `{"title":"","company":"","grade":"","tech_tags":[],"opened_date":null,
	"salary_from":null,"salary_to":null,"salary_currency":null,"salary_gross":null,"work_format":""}`

	result, err := Parse([]byte(empty))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !result.IsEmpty() {
		t.Errorf("Filled = %v, ожидалось пусто", result.Filled)
	}
	if len(result.Notes) != 0 {
		t.Errorf("замечания на пустом ответе: %+v", result.Notes)
	}
}

func TestParseBadFieldDoesNotKillOthers(t *testing.T) {
	// Главное свойство разбора: одно негодное поле не обнуляет остальные.
	answer := `{
	  "title": "Go-разработчик",
	  "company": "ООО Пример",
	  "grade": "боженька",
	  "tech_tags": ["Go"],
	  "opened_date": "01.08.2026",
	  "salary_from": -100,
	  "salary_to": 450000,
	  "salary_currency": "рубли",
	  "salary_gross": null,
	  "work_format": "из дома"
	}`

	result, err := Parse([]byte(answer))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// Годное сохранилось.
	if result.Values.Title != "Go-разработчик" {
		t.Errorf("title = %q", result.Values.Title)
	}
	if result.Values.SalaryTo == nil || *result.Values.SalaryTo != 450000 {
		t.Errorf("salary_to = %v, ожидалось 450000", result.Values.SalaryTo)
	}

	// Негодное отброшено и объяснено.
	for _, field := range []string{"grade", "opened_date", "salary_from", "salary_currency", "work_format"} {
		if result.Filled[field] {
			t.Errorf("поле %q отмечено заполненным, хотя значение негодное", field)
		}
		if !hasNote(result.Notes, field) {
			t.Errorf("нет замечания по полю %q: %+v", field, result.Notes)
		}
	}
	if result.Values.Grade != "" || result.Values.WorkFormat != "" {
		t.Errorf("негодные значения набора просочились: %+v", result.Values)
	}
}

func TestParseEnumNormalization(t *testing.T) {
	cases := map[string]string{
		`{"grade":"Middle"}`: model.GradeMiddle,
		`{"grade":"SENIOR"}`: model.GradeSenior,
		`{"grade":"lead"}`:   model.GradeLead,
	}

	for answer, want := range cases {
		result, err := Parse([]byte(answer))
		if err != nil {
			t.Fatalf("Parse(%s): %v", answer, err)
		}
		if result.Values.Grade != want {
			t.Errorf("Parse(%s): grade = %q, ожидалось %q", answer, result.Values.Grade, want)
		}
	}
}

func TestParsePlaceholdersTreatedAsEmpty(t *testing.T) {
	// Модели любят писать «не указано» вместо пустой строки. Смысл тот же,
	// поэтому такие значения приравниваются к пустым молча, без замечаний.
	for _, value := range []string{"не указано", "N/A", "unknown", "—", "null", "нет данных"} {
		answer := `{"company":"` + value + `","title":"Go-разработчик"}`

		result, err := Parse([]byte(answer))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if result.Values.Company != "" {
			t.Errorf("значение %q не распознано как пустое: %q", value, result.Values.Company)
		}
		if result.Filled["company"] {
			t.Errorf("значение %q отмечено как заполненное", value)
		}
		if hasNote(result.Notes, "company") {
			t.Errorf("значение %q дало лишнее замечание: %+v", value, result.Notes)
		}
	}
}

func TestParseSalaryTolerance(t *testing.T) {
	cases := []struct {
		name    string
		answer  string
		want    float64
		wantOK  bool
		wantSay bool // ожидается ли замечание
	}{
		{name: "число", answer: `{"salary_from":300000}`, want: 300000, wantOK: true},
		{name: "строка с пробелами", answer: `{"salary_from":"300 000"}`, want: 300000, wantOK: true, wantSay: true},
		{name: "строка с валютой", answer: `{"salary_from":"300000 ₽"}`, want: 300000, wantOK: true, wantSay: true},
		{name: "тысячи", answer: `{"salary_from":"300 тыс"}`, want: 300000, wantOK: true, wantSay: true},
		{name: "ноль это не вилка", answer: `{"salary_from":0}`, wantOK: false},
		{name: "отрицательная", answer: `{"salary_from":-5}`, wantOK: false, wantSay: true},
		{name: "неправдоподобная", answer: `{"salary_from":1e12}`, wantOK: false, wantSay: true},
		{name: "по договорённости", answer: `{"salary_from":"по договорённости"}`, wantOK: false, wantSay: true},
		{name: "null", answer: `{"salary_from":null}`, wantOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Parse([]byte(tc.answer))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			got := result.Values.SalaryFrom
			if !tc.wantOK {
				if got != nil {
					t.Errorf("salary_from = %v, ожидался nil", *got)
				}
			} else {
				if got == nil {
					t.Fatalf("salary_from = nil, ожидалось %v", tc.want)
				}
				if *got != tc.want {
					t.Errorf("salary_from = %v, ожидалось %v", *got, tc.want)
				}
			}

			if tc.wantSay && !hasNote(result.Notes, "salary_from") {
				t.Errorf("ожидалось замечание: %+v", result.Notes)
			}
			if !tc.wantSay && hasNote(result.Notes, "salary_from") {
				t.Errorf("лишнее замечание: %+v", result.Notes)
			}
		})
	}
}

func TestParseSalaryContradictionDropsBoth(t *testing.T) {
	// Если нижняя граница больше верхней, непонятно, какая из них выдумана.
	result, err := Parse([]byte(`{"salary_from":500000,"salary_to":100000,"salary_currency":"RUB"}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if result.Values.SalaryFrom != nil || result.Values.SalaryTo != nil {
		t.Errorf("вилка не отброшена: %v / %v", result.Values.SalaryFrom, result.Values.SalaryTo)
	}
	if !hasNote(result.Notes, "salary_from") {
		t.Errorf("нет объяснения: %+v", result.Notes)
	}
	// Валюта без вилки не имеет смысла и тоже не предлагается.
	if result.Filled["salary_currency"] {
		t.Error("валюта предложена без вилки")
	}
}

func TestParseCurrencySymbols(t *testing.T) {
	cases := map[string]string{
		`{"salary_from":100000,"salary_currency":"₽"}`:   "RUB",
		`{"salary_from":100000,"salary_currency":"руб"}`: "RUB",
		`{"salary_from":100000,"salary_currency":"$"}`:   "USD",
		`{"salary_from":100000,"salary_currency":"usd"}`: "USD",
	}

	for answer, want := range cases {
		result, err := Parse([]byte(answer))
		if err != nil {
			t.Fatalf("Parse(%s): %v", answer, err)
		}
		if result.Values.SalaryCurrency != want {
			t.Errorf("Parse(%s): валюта = %q, ожидалось %q", answer, result.Values.SalaryCurrency, want)
		}
	}
}

func TestParseCurrencyWithoutSalaryIgnored(t *testing.T) {
	result, err := Parse([]byte(`{"salary_currency":"RUB","salary_gross":true}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if result.Values.SalaryCurrency != "" || result.Values.SalaryGross != nil {
		t.Errorf("валюта и gross без вилки просочились: %+v", result.Values)
	}
}

func TestParseGrossTolerance(t *testing.T) {
	cases := map[string]*bool{
		`{"salary_from":1000,"salary_gross":true}`:        boolPtr(true),
		`{"salary_from":1000,"salary_gross":false}`:       boolPtr(false),
		`{"salary_from":1000,"salary_gross":"gross"}`:     boolPtr(true),
		`{"salary_from":1000,"salary_gross":"на руки"}`:   boolPtr(false),
		`{"salary_from":1000,"salary_gross":null}`:        nil,
		`{"salary_from":1000,"salary_gross":"непонятно"}`: nil,
	}

	for answer, want := range cases {
		result, err := Parse([]byte(answer))
		if err != nil {
			t.Fatalf("Parse(%s): %v", answer, err)
		}

		got := result.Values.SalaryGross
		switch {
		case want == nil && got != nil:
			t.Errorf("Parse(%s): gross = %v, ожидался nil", answer, *got)
		case want != nil && got == nil:
			t.Errorf("Parse(%s): gross = nil, ожидалось %v", answer, *want)
		case want != nil && got != nil && *want != *got:
			t.Errorf("Parse(%s): gross = %v, ожидалось %v", answer, *got, *want)
		}
	}
}

func TestParseTags(t *testing.T) {
	t.Run("дубликаты и пробелы", func(t *testing.T) {
		result, err := Parse([]byte(`{"tech_tags":[" Go ","go","PostgreSQL","","не указано","GO"]}`))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}

		want := []string{"Go", "PostgreSQL"}
		if len(result.Values.TechTags) != len(want) {
			t.Fatalf("tech_tags = %v, ожидалось %v", result.Values.TechTags, want)
		}
		for i, tag := range want {
			if result.Values.TechTags[i] != tag {
				t.Errorf("tech_tags[%d] = %q, ожидалось %q", i, result.Values.TechTags[i], tag)
			}
		}
	})

	t.Run("строка вместо массива", func(t *testing.T) {
		result, err := Parse([]byte(`{"tech_tags":"Go, PostgreSQL, Kubernetes"}`))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if len(result.Values.TechTags) != 3 {
			t.Errorf("tech_tags = %v", result.Values.TechTags)
		}
		if !hasNote(result.Notes, "tech_tags") {
			t.Error("нет замечания о неверном типе")
		}
	})

	t.Run("слишком много тегов", func(t *testing.T) {
		many := make([]string, 0, 60)
		for i := 0; i < 60; i++ {
			many = append(many, "tag"+string(rune('a'+i%26))+string(rune('0'+i/26)))
		}
		answer := `{"tech_tags":["` + strings.Join(many, `","`) + `"]}`

		result, err := Parse([]byte(answer))
		if err != nil {
			t.Fatalf("Parse: %v", err)
		}
		if len(result.Values.TechTags) > maxTags {
			t.Errorf("тегов %d, лимит %d", len(result.Values.TechTags), maxTags)
		}
		if !hasNote(result.Notes, "tech_tags") {
			t.Error("нет замечания об обрезке списка")
		}
	})
}

func TestParseDateSanity(t *testing.T) {
	cases := []struct {
		name   string
		answer string
		want   string
	}{
		{name: "нормальная дата", answer: `{"opened_date":"2026-08-01"}`, want: "2026-08-01"},
		{name: "битый формат", answer: `{"opened_date":"01.08.2026"}`, want: ""},
		{name: "из прошлого века", answer: `{"opened_date":"1970-01-01"}`, want: ""},
		{name: "далёкое будущее", answer: `{"opened_date":"2099-01-01"}`, want: ""},
		{name: "мусор", answer: `{"opened_date":"недавно"}`, want: ""},
		{name: "null", answer: `{"opened_date":null}`, want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Parse([]byte(tc.answer))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}

			if tc.want == "" {
				if result.Values.OpenedDate != nil {
					t.Errorf("opened_date = %s, ожидался nil", result.Values.OpenedDate)
				}
				return
			}
			if result.Values.OpenedDate == nil || result.Values.OpenedDate.String() != tc.want {
				t.Errorf("opened_date = %v, ожидалось %s", result.Values.OpenedDate, tc.want)
			}
		})
	}
}

func TestParseUnknownFieldNoted(t *testing.T) {
	result, err := Parse([]byte(`{"title":"Go-разработчик","city":"Москва"}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if !hasNote(result.Notes, "city") {
		t.Errorf("нет замечания о лишнем поле: %+v", result.Notes)
	}
	// При этом остальное сохраняется.
	if result.Values.Title != "Go-разработчик" {
		t.Errorf("title = %q", result.Values.Title)
	}
}

func TestParseWrappedAndDirtyJSON(t *testing.T) {
	cases := []struct {
		name   string
		answer string
	}{
		{name: "в markdown-заборе", answer: "```json\n" + goodAnswer + "\n```"},
		{name: "с пояснением до", answer: "Вот данные вакансии:\n" + goodAnswer},
		{name: "с пояснением после", answer: goodAnswer + "\n\nНадеюсь, это помогло!"},
		{name: "с отступами", answer: "   \n\t" + goodAnswer + "  \n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Parse([]byte(tc.answer))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if result.Values.Title != "Go-разработчик" {
				t.Errorf("title = %q", result.Values.Title)
			}
		})
	}
}

func TestParseNoJSON(t *testing.T) {
	cases := []struct {
		name   string
		answer string
	}{
		{name: "пусто", answer: ""},
		{name: "только текст", answer: "Извините, я не могу обработать эту страницу."},
		{name: "обрезанный json", answer: `{"title": "Go-разработчик"`},
		{name: "массив вместо объекта", answer: `["Go-разработчик"]`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse([]byte(tc.answer))
			if err == nil {
				t.Fatal("ожидалась ошибка")
			}
			// Только эта ошибка даёт право на повторную попытку.
			if !errors.Is(err, ErrNoJSON) {
				t.Errorf("err = %v, ожидалась ErrNoJSON", err)
			}
		})
	}
}

func TestParseLongTextTruncated(t *testing.T) {
	long := strings.Repeat("а", maxTextLen+50)
	result, err := Parse([]byte(`{"title":"` + long + `"}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len([]rune(result.Values.Title)) != maxTextLen {
		t.Errorf("длина title = %d, ожидалось %d", len([]rune(result.Values.Title)), maxTextLen)
	}
	if !hasNote(result.Notes, "title") {
		t.Error("нет замечания об обрезке")
	}
}

func TestParseCollapsesWhitespaceInTitle(t *testing.T) {
	result, err := Parse([]byte(`{"title":"Go-разработчик\n\n  (Москва)"}`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if result.Values.Title != "Go-разработчик (Москва)" {
		t.Errorf("title = %q", result.Values.Title)
	}
}

func hasNote(notes []FieldNote, field string) bool {
	for _, note := range notes {
		if note.Field == field {
			return true
		}
	}
	return false
}

func boolPtr(v bool) *bool { return &v }
