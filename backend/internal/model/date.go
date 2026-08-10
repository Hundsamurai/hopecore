package model

import (
	"database/sql/driver"
	"fmt"
	"strings"
	"time"
)

// DateLayout — формат хранения и представления календарных дат.
const DateLayout = "2006-01-02"

// Date — календарная дата без времени и таймзоны (поля opened_date, sent_at в ТЗ).
//
// Обычный time.Time здесь неудобен: он тянет за собой время и зону, из-за чего
// в API появлялись бы значения вида "2026-08-10T00:00:00Z", а сравнение дат
// зависело бы от таймзоны контейнера. Поэтому дата хранится в БД как TEXT
// "YYYY-MM-DD" и в JSON выглядит так же.
//
// Отсутствие даты выражается через *Date == nil, а не через нулевое значение.
type Date struct {
	time.Time
}

// NewDate собирает Date из года, месяца и дня в UTC.
func NewDate(year int, month time.Month, day int) Date {
	return Date{time.Date(year, month, day, 0, 0, 0, 0, time.UTC)}
}

// ParseDate разбирает дату в формате YYYY-MM-DD.
func ParseDate(s string) (Date, error) {
	t, err := time.ParseInLocation(DateLayout, s, time.UTC)
	if err != nil {
		return Date{}, fmt.Errorf("дата %q: ожидается формат %s", s, DateLayout)
	}
	return Date{t}, nil
}

// GormDataType задаёт тип колонки при миграции.
func (Date) GormDataType() string {
	return "text"
}

// String возвращает дату в формате YYYY-MM-DD.
func (d Date) String() string {
	return d.Format(DateLayout)
}

// Value сериализует дату для записи в БД.
func (d Date) Value() (driver.Value, error) {
	if d.IsZero() {
		return nil, nil
	}
	return d.Format(DateLayout), nil
}

// Scan читает дату из БД. Драйвер может отдать как строку, так и time.Time —
// поддерживаются оба варианта.
func (d *Date) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*d = Date{}
		return nil
	case time.Time:
		*d = NewDate(v.Year(), v.Month(), v.Day())
		return nil
	case []byte:
		return d.parse(string(v))
	case string:
		return d.parse(v)
	default:
		return fmt.Errorf("дата: неожиданный тип %T", src)
	}
}

func (d *Date) parse(s string) error {
	s = strings.TrimSpace(s)
	if s == "" {
		*d = Date{}
		return nil
	}
	// Подстраховка на случай, если в колонке лежит полная временная метка
	// (например, запись сделана в обход этого типа).
	if len(s) > len(DateLayout) {
		s = s[:len(DateLayout)]
	}
	parsed, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}

// MarshalJSON отдаёт дату строкой "YYYY-MM-DD".
func (d Date) MarshalJSON() ([]byte, error) {
	if d.IsZero() {
		return []byte("null"), nil
	}
	return []byte(`"` + d.Format(DateLayout) + `"`), nil
}

// UnmarshalJSON принимает строку "YYYY-MM-DD" или null.
func (d *Date) UnmarshalJSON(raw []byte) error {
	s := strings.Trim(string(raw), `"`)
	if s == "null" || s == "" {
		*d = Date{}
		return nil
	}
	parsed, err := ParseDate(s)
	if err != nil {
		return err
	}
	*d = parsed
	return nil
}
