package model

import (
	"encoding/json"
	"testing"
	"time"
)

func TestDateValueScanRoundTrip(t *testing.T) {
	in := NewDate(2026, time.August, 10)

	raw, err := in.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if got, want := raw, "2026-08-10"; got != want {
		t.Fatalf("Value = %v, ожидалось %q", got, want)
	}

	var got Date
	if err := got.Scan(raw); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if got.String() != in.String() {
		t.Errorf("после round-trip = %s, ожидалось %s", got, in)
	}
}

func TestDateValueZeroIsNull(t *testing.T) {
	raw, err := Date{}.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if raw != nil {
		t.Errorf("Value = %v, ожидался nil для нулевой даты", raw)
	}
}

func TestDateScan(t *testing.T) {
	cases := []struct {
		name    string
		src     any
		want    string
		wantErr bool
	}{
		{name: "строка", src: "2026-01-02", want: "2026-01-02"},
		{name: "байты", src: []byte("2026-01-02"), want: "2026-01-02"},
		{name: "time.Time от драйвера", src: time.Date(2026, time.March, 4, 15, 4, 5, 0, time.UTC), want: "2026-03-04"},
		{name: "полная метка времени в колонке", src: "2026-05-06T00:00:00Z", want: "2026-05-06"},
		{name: "NULL", src: nil, want: ""},
		{name: "пустая строка", src: "", want: ""},
		{name: "мусор", src: "не дата", wantErr: true},
		{name: "неподдерживаемый тип", src: 20260102, wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var got Date
			err := got.Scan(tc.src)

			if tc.wantErr {
				if err == nil {
					t.Fatalf("ожидалась ошибка, получено %s", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("Scan: %v", err)
			}
			if tc.want == "" {
				if !got.IsZero() {
					t.Errorf("= %s, ожидалась нулевая дата", got)
				}
				return
			}
			if got.String() != tc.want {
				t.Errorf("= %s, ожидалось %s", got, tc.want)
			}
		})
	}
}

func TestDateJSON(t *testing.T) {
	type payload struct {
		OpenedDate *Date `json:"opened_date"`
	}

	d := NewDate(2026, time.December, 31)
	raw, err := json.Marshal(payload{OpenedDate: &d})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if got, want := string(raw), `{"opened_date":"2026-12-31"}`; got != want {
		t.Fatalf("Marshal = %s, ожидалось %s", got, want)
	}

	var decoded payload
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if decoded.OpenedDate == nil || decoded.OpenedDate.String() != "2026-12-31" {
		t.Fatalf("Unmarshal = %v, ожидалось 2026-12-31", decoded.OpenedDate)
	}

	var empty payload
	if err := json.Unmarshal([]byte(`{"opened_date":null}`), &empty); err != nil {
		t.Fatalf("Unmarshal null: %v", err)
	}
	if empty.OpenedDate != nil {
		t.Errorf("opened_date = %v, ожидался nil", empty.OpenedDate)
	}
}

func TestIsValidGrade(t *testing.T) {
	cases := map[string]bool{
		"":       true, // грейд можно не указывать
		"junior": true,
		"senior": true,
		"lead":   true,
		"Senior": false,
		"god":    false,
	}

	for grade, want := range cases {
		if got := IsValidGrade(grade); got != want {
			t.Errorf("IsValidGrade(%q) = %v, ожидалось %v", grade, got, want)
		}
	}
}
