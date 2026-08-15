package fetcher

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// content — то, что удалось вытащить из разметки.
type content struct {
	title       string
	description string
	// structured — блоки application/ld+json с описанием вакансии (schema.org
	// JobPosting). Встречаются не везде, но где есть — это самые точные данные
	// на странице, и отдавать их модели дешевле, чем заставлять её угадывать.
	structured []string
	body       string
	text       string
	// antibot — на странице нашлись признаки проверки браузера.
	antibot bool
}

// skipTags — элементы, содержимое которых в текст не попадает.
//
// nav, footer и aside выброшены осознанно: это меню, подвал и блоки
// «похожие вакансии», из которых модель начинает собирать чужие данные.
// header оставлен: там обычно название должности.
var skipTags = map[atom.Atom]bool{
	atom.Script:   true,
	atom.Style:    true,
	atom.Noscript: true,
	atom.Svg:      true,
	atom.Template: true,
	atom.Iframe:   true,
	atom.Nav:      true,
	atom.Footer:   true,
	atom.Aside:    true,
	atom.Form:     true,
	atom.Select:   true,
	atom.Option:   true,
}

// blockTags — элементы, вокруг которых нужен перевод строки, иначе абзацы
// и пункты списка склеиваются в одно слово.
var blockTags = map[atom.Atom]bool{
	atom.P: true, atom.Div: true, atom.Br: true, atom.Li: true,
	atom.Tr: true, atom.Section: true, atom.Article: true, atom.Header: true,
	atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true,
	atom.Ul: true, atom.Ol: true, atom.Table: true, atom.Dl: true, atom.Dt: true, atom.Dd: true,
}

// antibotMarkers — признаки страницы-заглушки вместо вакансии.
// Список собран по реальным ответам: ozon.tech отдаёт «Antibot Challenge Page».
var antibotMarkers = []string{
	"antibot",
	"challenge page",
	"captcha",
	"каптча",
	"проверка браузера",
	"checking your browser",
	"enable javascript",
	"включите javascript",
	"attention required",
}

// extract разбирает HTML и собирает текст для модели.
//
// Используется настоящий парсер, а не регулярные выражения: разметка на живых
// сайтах сломана в самых неожиданных местах, а парсер разбирает её так же,
// как браузер, и корректно отбрасывает содержимое script и style.
func extract(raw string) content {
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		// Парсер stdlib почти не ошибается, но если это случилось — лучше отдать
		// сырую разметку без тегов, чем ничего.
		fallback := strings.TrimSpace(collapse(stripTags(raw)))
		return content{body: fallback, text: fallback, antibot: hasAntibotMarker(fallback)}
	}

	var c content
	var body strings.Builder

	walk(doc, &c, &body)

	c.body = strings.TrimSpace(collapse(body.String()))
	c.text = assemble(c)
	c.antibot = hasAntibotMarker(c.text)
	return c
}

func walk(node *html.Node, c *content, body *strings.Builder) {
	switch node.Type {
	case html.ElementNode:
		if node.DataAtom == atom.Title && c.title == "" {
			c.title = strings.TrimSpace(collapse(textOf(node)))
			return
		}
		if node.DataAtom == atom.Meta {
			readMeta(node, c)
			return
		}
		if node.DataAtom == atom.Script {
			// Скрипты в текст не идут, но ld+json — исключение: это данные.
			if isJobPostingLD(node) {
				c.structured = append(c.structured, strings.TrimSpace(textOf(node)))
			}
			return
		}
		if skipTags[node.DataAtom] {
			return
		}
		if blockTags[node.DataAtom] {
			body.WriteString("\n")
		}

	case html.TextNode:
		body.WriteString(node.Data)

	case html.CommentNode:
		return
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		walk(child, c, body)
	}

	if node.Type == html.ElementNode && blockTags[node.DataAtom] {
		body.WriteString("\n")
	}
}

// readMeta забирает описание страницы: og:description у сайтов вакансий
// нередко содержит вилку и город одной строкой.
func readMeta(node *html.Node, c *content) {
	var name, property, contentAttr string
	for _, attr := range node.Attr {
		switch strings.ToLower(attr.Key) {
		case "name":
			name = strings.ToLower(attr.Val)
		case "property":
			property = strings.ToLower(attr.Val)
		case "content":
			contentAttr = attr.Val
		}
	}

	if contentAttr == "" {
		return
	}
	if name == "description" || property == "og:description" {
		if c.description == "" {
			c.description = strings.TrimSpace(collapse(contentAttr))
		}
	}
	// og:title используем, только если обычного <title> не нашлось.
	if property == "og:title" && c.title == "" {
		c.title = strings.TrimSpace(collapse(contentAttr))
	}
}

func isJobPostingLD(node *html.Node) bool {
	isLD := false
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, "type") && strings.Contains(strings.ToLower(attr.Val), "ld+json") {
			isLD = true
		}
	}
	if !isLD {
		return false
	}
	// Берём только разметку вакансии: разметка хлебных крошек и организации
	// в запросе к модели была бы шумом.
	return strings.Contains(textOf(node), "JobPosting")
}

// assemble складывает части в один текст. Порядок важен: заголовок и описание
// идут первыми, потому что при обрезке по лимиту сохраняется начало.
func assemble(c content) string {
	var parts []string

	if c.title != "" {
		parts = append(parts, "Заголовок страницы: "+c.title)
	}
	if c.description != "" {
		parts = append(parts, "Описание из метаданных: "+c.description)
	}
	for _, block := range c.structured {
		parts = append(parts, "Структурированные данные вакансии (schema.org):\n"+block)
	}
	if c.body != "" {
		parts = append(parts, "Текст страницы:\n"+c.body)
	}

	return strings.Join(parts, "\n\n")
}

func textOf(node *html.Node) string {
	var sb strings.Builder
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.TextNode {
			sb.WriteString(child.Data)
			continue
		}
		sb.WriteString(textOf(child))
	}
	return sb.String()
}

// collapse сжимает пробелы, но сохраняет переводы строк: абзацы помогают
// модели отличать разделы вакансии друг от друга.
func collapse(text string) string {
	var sb strings.Builder
	sb.Grow(len(text))

	lastSpace := false
	lastNewline := false

	for _, r := range text {
		switch {
		case r == '\n' || r == '\r':
			if !lastNewline {
				sb.WriteRune('\n')
			}
			lastNewline = true
			lastSpace = true
		case r == ' ' || r == '\t' || r == '\u00a0' || r == '\u200b':
			if !lastSpace {
				sb.WriteRune(' ')
			}
			lastSpace = true
		default:
			sb.WriteRune(r)
			lastSpace = false
			lastNewline = false
		}
	}

	// Пустые строки, оставшиеся от вложенных блоков, склеиваем.
	lines := strings.Split(sb.String(), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	return strings.Join(kept, "\n")
}

// stripTags — грубое удаление разметки для случая, когда парсер не справился.
func stripTags(raw string) string {
	var sb strings.Builder
	inTag := false
	for _, r := range raw {
		switch {
		case r == '<':
			inTag = true
			sb.WriteRune(' ')
		case r == '>':
			inTag = false
		case !inTag:
			sb.WriteRune(r)
		}
	}
	return html.UnescapeString(sb.String())
}

func hasAntibotMarker(text string) bool {
	lower := strings.ToLower(text)
	for _, marker := range antibotMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
