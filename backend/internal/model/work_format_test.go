package model

import "testing"

func TestIsValidWorkFormat(t *testing.T) {
	cases := map[string]bool{
		"":        true, // формат можно не указывать
		"onsite":  true,
		"hybrid":  true,
		"remote":  true,
		"Remote":  false,
		"из дома": false,
		"office":  false,
	}

	for format, want := range cases {
		if got := IsValidWorkFormat(format); got != want {
			t.Errorf("IsValidWorkFormat(%q) = %v, ожидалось %v", format, got, want)
		}
	}
}

func TestWorkFormatsSet(t *testing.T) {
	if len(WorkFormats) != 3 {
		t.Errorf("форматов работы: %d, ожидалось 3", len(WorkFormats))
	}
	for _, format := range WorkFormats {
		if !IsValidWorkFormat(format) {
			t.Errorf("формат %q из набора не проходит валидацию", format)
		}
	}
}
