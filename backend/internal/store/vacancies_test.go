package store

import (
	"testing"

	"github.com/Hundsamurai/hopecore/backend/internal/activity"
	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

func boolPtr(v bool) *bool { return &v }

// TestActiveFilterSQLMatchesResolve страхует от расхождения двух реализаций
// одного правила: ActiveFilterSQL в БД и activity.Resolve в Go.
func TestActiveFilterSQLMatchesResolve(t *testing.T) {
	db := newMemoryDB(t)

	states := []*bool{nil, boolPtr(true), boolPtr(false)}

	type expectation struct {
		id     uint
		active bool
	}
	var expectations []expectation

	for _, auto := range states {
		for _, manual := range states {
			vacancy := model.Vacancy{
				URL:            "https://example.com/vacancy",
				AutoIsActive:   auto,
				ManualIsActive: manual,
			}
			if err := CreateVacancy(db, &vacancy); err != nil {
				t.Fatalf("создание вакансии: %v", err)
			}

			active, _ := activity.Resolve(auto, manual)
			expectations = append(expectations, expectation{id: vacancy.ID, active: active})
		}
	}

	visible, err := ListVacancies(db, VacancyFilter{})
	if err != nil {
		t.Fatalf("ListVacancies: %v", err)
	}

	visibleIDs := make(map[uint]bool, len(visible))
	for _, v := range visible {
		visibleIDs[v.ID] = true
	}

	for _, want := range expectations {
		if visibleIDs[want.id] != want.active {
			t.Errorf("вакансия %d: SQL-фильтр показывает %v, activity.Resolve говорит %v",
				want.id, visibleIDs[want.id], want.active)
		}
	}

	all, err := ListVacancies(db, VacancyFilter{IncludeInactive: true})
	if err != nil {
		t.Fatalf("ListVacancies(include_inactive): %v", err)
	}
	if len(all) != len(expectations) {
		t.Errorf("с include_inactive отдано %d записей, ожидалось %d", len(all), len(expectations))
	}
}

func TestListVacanciesRejectsUnknownSort(t *testing.T) {
	db := newMemoryDB(t)

	// Имя колонки подставляется в SQL, поэтому белый список обязателен.
	if _, err := ListVacancies(db, VacancyFilter{Sort: "id; DROP TABLE vacancies"}); err == nil {
		t.Fatal("сортировка по произвольному выражению принята, ожидалась ошибка")
	}

	if !db.Migrator().HasTable("vacancies") {
		t.Fatal("таблица vacancies пропала")
	}
}

func TestSortableFieldsWhitelist(t *testing.T) {
	if !IsSortableField("updated_at") {
		t.Error("updated_at должен быть разрешён: это сортировка по умолчанию")
	}
	if IsSortableField("last_check_error") {
		t.Error("last_check_error не должен попадать в белый список")
	}
	if len(SortableFields()) == 0 {
		t.Error("список полей сортировки пуст")
	}
}

func TestGetVacancyNotFound(t *testing.T) {
	db := newMemoryDB(t)

	if _, err := GetVacancy(db, 12345); err == nil {
		t.Fatal("ожидалась ошибка ErrNotFound")
	}
}

func TestDeleteVacancyNotFound(t *testing.T) {
	db := newMemoryDB(t)

	if err := DeleteVacancy(db, 12345); err != ErrNotFound {
		t.Fatalf("err = %v, ожидалась ErrNotFound", err)
	}
}
