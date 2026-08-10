package activity

import "testing"

func TestResolve(t *testing.T) {
	cases := []struct {
		name         string
		auto         *bool
		manual       *bool
		wantActive   bool
		wantConflict bool
	}{
		{
			name:       "ничего не известно — считаем активной",
			auto:       nil,
			manual:     nil,
			wantActive: true,
		},
		{
			name:       "проверка сказала активна",
			auto:       ptr(true),
			manual:     nil,
			wantActive: true,
		},
		{
			name:       "проверка сказала снята",
			auto:       ptr(false),
			manual:     nil,
			wantActive: false,
		},
		{
			name:       "override активна, проверок не было",
			auto:       nil,
			manual:     ptr(true),
			wantActive: true,
		},
		{
			name:       "override снята, проверок не было",
			auto:       nil,
			manual:     ptr(false),
			wantActive: false,
		},
		{
			name:       "override и проверка согласны: активна",
			auto:       ptr(true),
			manual:     ptr(true),
			wantActive: true,
		},
		{
			name:       "override и проверка согласны: снята",
			auto:       ptr(false),
			manual:     ptr(false),
			wantActive: false,
		},
		{
			name:         "сайт отдаёт 200, но пользователь закрыл вакансию",
			auto:         ptr(true),
			manual:       ptr(false),
			wantActive:   false,
			wantConflict: true,
		},
		{
			name:         "сайт отдал 404, но пользователь считает вакансию живой",
			auto:         ptr(false),
			manual:       ptr(true),
			wantActive:   true,
			wantConflict: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			active, conflict := Resolve(tc.auto, tc.manual)

			if active != tc.wantActive {
				t.Errorf("effective = %v, ожидалось %v", active, tc.wantActive)
			}
			if conflict != tc.wantConflict {
				t.Errorf("conflict = %v, ожидалось %v", conflict, tc.wantConflict)
			}
		})
	}
}

func TestResolveManualWins(t *testing.T) {
	// Ключевое требование этапа: авто-проверка не перебивает решение пользователя.
	for _, auto := range []*bool{nil, ptr(true), ptr(false)} {
		for _, manual := range []*bool{ptr(true), ptr(false)} {
			active, _ := Resolve(auto, manual)
			if active != *manual {
				t.Errorf("auto=%v manual=%v: effective = %v, ожидалось решение пользователя %v",
					auto, *manual, active, *manual)
			}
		}
	}
}
