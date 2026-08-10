package api

import "encoding/json"

// Optional различает три состояния поля в JSON-запросе: поле отсутствует,
// поле пришло со значением null, поле пришло со значением.
//
// Это нужно для PATCH: «не трогать поле» и «очистить поле» — разные операции,
// а обычный указатель их не различает.
type Optional[T any] struct {
	// Set — поле присутствовало в JSON.
	Set bool
	// Value == nil при Set == true означает явный null (очистить поле).
	Value *T
}

// UnmarshalJSON реализует json.Unmarshaler.
func (o *Optional[T]) UnmarshalJSON(raw []byte) error {
	o.Set = true

	if string(raw) == "null" {
		o.Value = nil
		return nil
	}

	var value T
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	o.Value = &value
	return nil
}

// Provided сообщает, что поле пришло с непустым значением.
func (o Optional[T]) Provided() bool {
	return o.Set && o.Value != nil
}

// Cleared сообщает, что поле пришло явным null.
func (o Optional[T]) Cleared() bool {
	return o.Set && o.Value == nil
}
