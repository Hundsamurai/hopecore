package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// Tags — список тегов технологий вакансии (п. 4.1 ТЗ).
//
// В SQLite нет массивов, поэтому значение хранится как JSON-строка в колонке TEXT
// (см. docs/main/design-stage1.md, п. 3). Тип реализует driver.Valuer и sql.Scanner,
// так что gorm работает с ним как с обычным полем.
type Tags []string

// GormDataType задаёт тип колонки при миграции.
func (Tags) GormDataType() string {
	return "text"
}

// Value сериализует теги для записи в БД. nil и пустой список одинаково
// превращаются в "[]": отдельный смысл у NULL здесь не нужен.
func (t Tags) Value() (driver.Value, error) {
	if t == nil {
		return "[]", nil
	}
	raw, err := json.Marshal([]string(t))
	if err != nil {
		return nil, fmt.Errorf("сериализация tech_tags: %w", err)
	}
	return string(raw), nil
}

// Scan читает теги из БД. Пустые значения и NULL дают пустой непустой-nil слайс,
// чтобы вызывающий код не проверял nil перед перебором.
func (t *Tags) Scan(src any) error {
	if src == nil {
		*t = Tags{}
		return nil
	}

	var raw []byte
	switch v := src.(type) {
	case []byte:
		raw = v
	case string:
		raw = []byte(v)
	default:
		return fmt.Errorf("tech_tags: неожиданный тип %T", src)
	}

	if len(raw) == 0 {
		*t = Tags{}
		return nil
	}

	var parsed []string
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return fmt.Errorf("разбор tech_tags %q: %w", string(raw), err)
	}
	if parsed == nil {
		parsed = []string{}
	}
	*t = parsed
	return nil
}

// MarshalJSON гарантирует, что в API теги всегда массив, а не null.
func (t Tags) MarshalJSON() ([]byte, error) {
	if t == nil {
		return []byte("[]"), nil
	}
	return json.Marshal([]string(t))
}
