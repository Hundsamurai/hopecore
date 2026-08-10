// Package service содержит сценарии, которые связывают БД и внешний мир.
//
// Пакет activity намеренно оставлен без зависимости от store: правило вычисления
// активности и HTTP-проверка не должны ничего знать про БД. Оркестрация живёт здесь.
package service

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/Hundsamurai/hopecore/backend/internal/activity"
	"github.com/Hundsamurai/hopecore/backend/internal/model"
	"github.com/Hundsamurai/hopecore/backend/internal/store"
)

// CheckSummary — итог массовой проверки, который UI показывает одним сообщением.
type CheckSummary struct {
	// Checked — сколько вакансий реально опросили.
	Checked int `json:"checked"`
	// Skipped — сколько пропустили: они помечены неактивными вручную.
	Skipped int `json:"skipped"`
	// BecameInactive — сколько вакансий перешло из активных в снятые по итогам проверки.
	BecameInactive int `json:"became_inactive"`
	// Unknown — сайт ответил, но код не позволяет судить об активности (429, 5xx, 403).
	Unknown int `json:"unknown"`
	// Failed — ответа не было вовсе: таймаут, DNS, отказ соединения.
	Failed int `json:"failed"`
}

// ActivityService запускает проверки активности и правит ручной override.
type ActivityService struct {
	db          *gorm.DB
	checker     activity.Checker
	concurrency int
	log         *slog.Logger
	now         func() time.Time
}

// NewActivityService собирает сервис. concurrency < 1 трактуется как 1.
func NewActivityService(db *gorm.DB, checker activity.Checker, concurrency int, log *slog.Logger) *ActivityService {
	if concurrency < 1 {
		concurrency = 1
	}
	return &ActivityService{
		db:          db,
		checker:     checker,
		concurrency: concurrency,
		log:         log,
		now:         func() time.Time { return time.Now().UTC() },
	}
}

// CheckOne опрашивает одну вакансию и возвращает её обновлённое состояние.
func (s *ActivityService) CheckOne(ctx context.Context, id uint) (*model.Vacancy, error) {
	vacancy, err := store.GetVacancy(s.db, id)
	if err != nil {
		return nil, err
	}

	// По одной вакансии проверка запускается даже при ручном override:
	// пользователь нажал кнопку осознанно и вправе увидеть, что говорит сайт.
	// Override при этом не меняется, а расхождение отразится в activity_conflict.
	if err := s.check(ctx, vacancy); err != nil {
		return nil, err
	}

	return store.GetVacancy(s.db, id)
}

// CheckAll опрашивает все вакансии, кроме закрытых вручную.
//
// Крона нет: метод вызывается только по кнопке (п. 3 ТЗ).
func (s *ActivityService) CheckAll(ctx context.Context) (CheckSummary, error) {
	total, err := store.CountVacancies(s.db)
	if err != nil {
		return CheckSummary{}, err
	}

	checkable, err := store.ListCheckableVacancies(s.db)
	if err != nil {
		return CheckSummary{}, err
	}

	summary := CheckSummary{Skipped: total - len(checkable)}

	tasks := make(chan model.Vacancy)
	results := make(chan checkOutcome, len(checkable))

	var wg sync.WaitGroup
	for i := 0; i < s.concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for vacancy := range tasks {
				results <- s.checkForSummary(ctx, vacancy)
			}
		}()
	}

	go func() {
		defer close(tasks)
		for _, vacancy := range checkable {
			select {
			case <-ctx.Done():
				return
			case tasks <- vacancy:
			}
		}
	}()

	wg.Wait()
	close(results)

	for outcome := range results {
		summary.Checked++
		switch {
		case outcome.failed:
			summary.Failed++
		case outcome.unknown:
			summary.Unknown++
		case outcome.becameInactive:
			summary.BecameInactive++
		}
	}

	s.log.Info("массовая проверка активности завершена",
		"checked", summary.Checked,
		"skipped", summary.Skipped,
		"became_inactive", summary.BecameInactive,
		"unknown", summary.Unknown,
		"failed", summary.Failed,
	)

	return summary, ctx.Err()
}

// SetManualActivity задаёт (true/false) или снимает (nil) ручной override.
func (s *ActivityService) SetManualActivity(ctx context.Context, id uint, manual *bool) (*model.Vacancy, error) {
	if err := store.SetManualActivity(s.db, id, manual); err != nil {
		return nil, err
	}

	s.log.Info("ручной статус активности обновлён", "id", id, "manual_is_active", describeOverride(manual))
	return store.GetVacancy(s.db, id)
}

type checkOutcome struct {
	unknown        bool
	failed         bool
	becameInactive bool
}

// check выполняет проверку и сохраняет результат.
func (s *ActivityService) check(ctx context.Context, vacancy *model.Vacancy) error {
	res := s.checker.Check(ctx, vacancy.URL)

	if res.Err != "" {
		s.log.Warn("проверка активности не дала результата",
			"id", vacancy.ID,
			"url", vacancy.URL,
			"status_code", describeStatus(res.StatusCode),
			"reason", res.Err,
		)
	}

	return store.SaveCheckResult(s.db, vacancy.ID, res.Active, s.now(), res.StatusCode, res.Err)
}

// checkForSummary повторяет check, но описывает исход в терминах сводки.
func (s *ActivityService) checkForSummary(ctx context.Context, vacancy model.Vacancy) checkOutcome {
	wasActive, _ := activity.Resolve(vacancy.AutoIsActive, vacancy.ManualIsActive)

	res := s.checker.Check(ctx, vacancy.URL)

	if err := store.SaveCheckResult(s.db, vacancy.ID, res.Active, s.now(), res.StatusCode, res.Err); err != nil {
		s.log.Error("не удалось сохранить результат проверки", "id", vacancy.ID, "error", err)
		return checkOutcome{failed: true}
	}

	if res.Unknown() {
		if res.Err != "" {
			s.log.Warn("проверка активности не дала результата",
				"id", vacancy.ID, "url", vacancy.URL, "status_code", describeStatus(res.StatusCode), "reason", res.Err)
		}
		// Ответ был, но код неинформативен — это «unknown».
		// Ответа не было вовсе — это «failed».
		if res.StatusCode == nil {
			return checkOutcome{failed: true}
		}
		return checkOutcome{unknown: true}
	}

	nowActive, _ := activity.Resolve(res.Active, vacancy.ManualIsActive)
	return checkOutcome{becameInactive: wasActive && !nowActive}
}

// describeOverride и describeStatus нужны, потому что slog печатает указатели
// адресами: в логе полезно видеть значение, а не 0x4000224170.
func describeOverride(manual *bool) string {
	switch {
	case manual == nil:
		return "нет (по авто-проверке)"
	case *manual:
		return "активна"
	default:
		return "неактивна"
	}
}

func describeStatus(statusCode *int) any {
	if statusCode == nil {
		return "нет ответа"
	}
	return *statusCode
}
