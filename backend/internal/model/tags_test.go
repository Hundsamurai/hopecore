package model

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestTagsValueScanRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		in   Tags
		want Tags
	}{
		{name: "порядок сохраняется", in: Tags{"go", "docker", "sqlite"}, want: Tags{"go", "docker", "sqlite"}},
		{name: "пустой список", in: Tags{}, want: Tags{}},
		{name: "nil превращается в пустой список", in: nil, want: Tags{}},
		{name: "юникод и пробелы", in: Tags{"C#", "Go 1.22", "Тест"}, want: Tags{"C#", "Go 1.22", "Тест"}},
		{name: "дубликаты не схлопываются", in: Tags{"go", "go"}, want: Tags{"go", "go"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := tc.in.Value()
			if err != nil {
				t.Fatalf("Value: %v", err)
			}

			stored, ok := raw.(string)
			if !ok {
				t.Fatalf("Value вернул %T, ожидалась строка", raw)
			}

			var got Tags
			if err := got.Scan(stored); err != nil {
				t.Fatalf("Scan: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("после round-trip = %#v, ожидалось %#v", got, tc.want)
			}
		})
	}
}

func TestTagsScanEdgeCases(t *testing.T) {
	cases := []struct {
		name    string
		src     any
		want    Tags
		wantErr bool
	}{
		{name: "NULL", src: nil, want: Tags{}},
		{name: "пустая строка", src: "", want: Tags{}},
		{name: "байты", src: []byte(`["go","k8s"]`), want: Tags{"go", "k8s"}},
		{name: "json null", src: "null", want: Tags{}},
		{name: "битый json", src: "[go", wantErr: true},
		{name: "неподдерживаемый тип", src: 42, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got Tags
			err := got.Scan(tc.src)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("ожидалась ошибка, получено %#v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Scan: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("= %#v, ожидалось %#v", got, tc.want)
			}
		})
	}
}

func TestTagsMarshalJSONNeverNull(t *testing.T) {
	// UI перебирает теги без проверок, поэтому в JSON всегда должен быть массив.
	raw, err := json.Marshal(struct {
		Tags Tags `json:"tech_tags"`
	}{})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got, want := string(raw), `{"tech_tags":[]}`; got != want {
		t.Errorf("= %s, ожидалось %s", got, want)
	}
}
