package store

import (
	"io"
	"log/slog"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// gormConfig собирает настройки gorm, общие для файловой и in-memory БД.
func gormConfig(log *slog.Logger) *gorm.Config {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return &gorm.Config{
		Logger: gormlogger.New(gormWriter{log: log}, gormlogger.Config{
			SlowThreshold: slowQueryThreshold,
			// Логируем только проблемы: поток SQL-запросов личному инструменту не нужен.
			LogLevel: gormlogger.Warn,
			// "record not found" — нормальный результат запроса по id, а не сбой.
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		}),
		// Даты и метки времени храним в UTC: приложение локальное, но так
		// значения в БД не зависят от таймзоны контейнера.
		NowFunc:                                  nowUTC,
		DisableForeignKeyConstraintWhenMigrating: false,
		TranslateError:                           true,
	}
}
