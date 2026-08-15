package llm

import (
	"fmt"
	"strings"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

// PromptVersion пишется в журнал запусков. Меняется вместе с текстом промпта
// или схемой: без этого нельзя объяснить, чем получен старый результат.
const PromptVersion = "extract-v1"

// ExtractionSchema описывает поля, которые модель должна найти на странице.
//
// Состав повторяет поля вакансии: извлекать то, что некуда сохранить,
// значит жечь токены впустую.
func ExtractionSchema() Schema {
	return Schema{
		Name:        "vacancy_extraction",
		Description: "Данные вакансии, извлечённые со страницы",
		Fields: []Field{
			{
				Name:        "title",
				Type:        TypeString,
				Description: "Должность как она названа в объявлении, например «Go-разработчик» или «Backend Engineer». Пустая строка, если на странице нет.",
			},
			{
				Name:        "company",
				Type:        TypeString,
				Description: "Название компании-работодателя. Пустая строка, если не указано.",
			},
			{
				Name: "grade",
				Type: TypeString,
				Enum: model.Grades,
				// Nullable, а не пустая строка в наборе: Gemini отвергает схему
				// с пустым значением в enum («enum[0]: cannot be empty»),
				// поэтому «не нашёл» выражается через null.
				Nullable:    true,
				Description: "Уровень позиции. Если прямо не назван, определи по требуемому опыту: до года — intern, 1-3 года — junior, 3-5 лет — middle, 5+ лет — senior. null, если судить не по чему.",
			},
			{
				Name:        "tech_tags",
				Type:        TypeStringArray,
				Description: "Технологии из требований: языки, базы данных, инструменты. Сначала главные. Только то, что названо на странице; пустой массив, если ничего нет.",
			},
			{
				Name:        "opened_date",
				Type:        TypeString,
				Nullable:    true,
				Description: "Дата публикации вакансии в формате YYYY-MM-DD. null, если на странице её нет. Не вычисляй из «опубликовано 3 дня назад».",
			},
			{
				Name:        "salary_from",
				Type:        TypeNumber,
				Nullable:    true,
				Description: "Нижняя граница зарплатной вилки числом, без пробелов и символа валюты. null, если вилки нет.",
			},
			{
				Name:        "salary_to",
				Type:        TypeNumber,
				Nullable:    true,
				Description: "Верхняя граница зарплатной вилки числом. null, если не указана.",
			},
			{
				Name:        "salary_currency",
				Type:        TypeString,
				Nullable:    true,
				Description: "Код валюты из трёх латинских букв: RUB, USD, EUR. null, если вилки нет.",
			},
			{
				Name:        "salary_gross",
				Type:        TypeBoolean,
				Nullable:    true,
				Description: "true, если зарплата указана до вычета налогов (gross, «до вычета»); false, если на руки (net, «на руки»); null, если не сказано.",
			},
			{
				Name:        "work_format",
				Type:        TypeString,
				Enum:        model.WorkFormats,
				Nullable:    true,
				Description: "Формат работы: onsite — в офисе, hybrid — гибрид, remote — удалённо. Переведи формулировку сайта в одно из этих значений; null, если формат не указан.",
			},
		},
	}
}

// SystemPrompt — правила поведения модели.
//
// Три главных: не выдумывать, переводить формулировки сайта в наши наборы
// значений и не путать вилку из объявления с личным предложением кандидату.
const SystemPrompt = `Ты извлекаешь данные о вакансии из текста веб-страницы.

Правила:
1. Не выдумывай. Если данных на странице нет, верни пустую строку, пустой массив или null. Пустой ответ полезнее выдуманного: пользователь заполнит поле сам.
2. Не догадывайся по косвенным признакам. Название компании берётся из текста, а не из домена; дата — только если она прямо написана.
3. Зарплата — это вилка из объявления. Если на странице написано «зарплата по договорённости», «достойная зарплата» или подобное без цифр, верни null.
4. Значения с ограниченным набором (grade, work_format) приводи к разрешённым: не копируй формулировку сайта дословно.
5. Если на странице несколько вакансий, извлекай ту, которой посвящена страница. Если понять невозможно, верни пустые значения.
6. Отвечай только JSON по заданной схеме, без пояснений и без markdown-разметки.`

// BuildUserPrompt собирает запрос с текстом страницы.
func BuildUserPrompt(sourceURL, pageText string) string {
	var sb strings.Builder

	sb.WriteString("Извлеки данные вакансии из текста страницы.\n\n")
	if sourceURL != "" {
		// Ссылка помогает узнать сайт, но правило 2 запрещает выводить
		// из неё название компании.
		sb.WriteString("Ссылка на страницу: " + sourceURL + "\n\n")
	}
	sb.WriteString("Текст страницы:\n---\n")
	sb.WriteString(pageText)
	sb.WriteString("\n---")

	return sb.String()
}

// RepairPrompt — добавка ко второй попытке, когда первый ответ не прошёл разбор.
// Нужна провайдерам без строгой схемы: DeepSeek гарантирует валидный JSON,
// но не соответствие схеме.
func RepairPrompt(problem string) string {
	return fmt.Sprintf(
		"Предыдущий ответ не подошёл: %s\nВерни только корректный JSON строго по схеме, без markdown и пояснений.",
		problem,
	)
}
