// Package store отвечает за подключение к SQLite, миграцию схемы и запросы к БД.
package store

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/Hundsamurai/hopecore/backend/internal/model"
)

// pragmas применяются к каждому соединению.
//
//	foreign_keys(1)    — без него SQLite молча игнорирует внешние ключи,
//	                     и каскадное удаление статуса кандидата не работает;
//	busy_timeout(5000) — ждать вместо мгновенной ошибки SQLITE_BUSY.
const pragmas = "_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)"

// journalWAL включается только для файловой БД: у in-memory журнал всегда "memory".
const journalWAL = "_pragma=journal_mode(WAL)"

// Open подключается к файловой БД по указанному пути, создавая каталог при необходимости.
func Open(path string, log *slog.Logger) (*gorm.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("путь к БД не задан")
	}

	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("создание каталога %s: %w", dir, err)
		}
	}

	dsn := "file:" + url.PathEscape(path) + "?" + pragmas + "&" + journalWAL

	db, err := gorm.Open(sqlite.Open(dsn), gormConfig(log))
	if err != nil {
		return nil, fmt.Errorf("подключение к БД %s: %w", path, err)
	}

	// Инструмент однопользовательский, а SQLite не любит параллельную запись:
	// одно соединение убирает проблему блокировок целиком.
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	return db, nil
}

// memoryCounter даёт уникальные имена in-memory базам, чтобы параллельные тесты
// не видели чужие таблицы.
var memoryCounter atomic.Uint64

// OpenMemory поднимает БД в памяти. Используется в тестах: cache=shared держит
// одну и ту же базу для всех соединений пула, а единственное соединение
// не даёт SQLite удалить её между запросами.
func OpenMemory(log *slog.Logger) (*gorm.DB, error) {
	name := fmt.Sprintf("hopecore-test-%d", memoryCounter.Add(1))
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared&%s", name, pragmas)

	db, err := gorm.Open(sqlite.Open(dsn), gormConfig(log))
	if err != nil {
		return nil, fmt.Errorf("подключение к in-memory БД: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)

	return db, nil
}

// Close закрывает пул соединений.
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// models перечисляет все сущности схемы. Порядок важен: сначала таблицы,
// на которые ссылаются внешние ключи.
//
// Все четыре таблицы создаются уже в MVP, хотя interview_summary и ai_block
// используются только на Этапе 3 — так позже не придётся возиться с миграциями.
func models() []any {
	return []any{
		&model.Vacancy{},
		&model.CandidateStatus{},
		&model.InterviewSummary{},
		&model.AIBlock{},
	}
}

// Migrate приводит схему БД в соответствие с моделями.
func Migrate(db *gorm.DB) error {
	if err := db.AutoMigrate(models()...); err != nil {
		return fmt.Errorf("миграция схемы: %w", err)
	}
	return nil
}

// TableNames возвращает имена таблиц схемы — удобно для проверок и диагностики.
func TableNames() []string {
	return []string{"vacancies", "candidate_status", "interview_summary", "ai_block"}
}

// PragmaString читает строковое значение pragma (например, journal_mode).
func PragmaString(db *gorm.DB, name string) (string, error) {
	if !isSafePragmaName(name) {
		return "", fmt.Errorf("недопустимое имя pragma: %q", name)
	}
	var value string
	// Имя pragma нельзя передать параметром, поэтому оно проверяется по белому списку символов.
	if err := db.Raw("PRAGMA " + name).Scan(&value).Error; err != nil {
		return "", err
	}
	return value, nil
}

// PragmaInt читает числовое значение pragma (например, foreign_keys).
func PragmaInt(db *gorm.DB, name string) (int, error) {
	if !isSafePragmaName(name) {
		return 0, fmt.Errorf("недопустимое имя pragma: %q", name)
	}
	var value int
	if err := db.Raw("PRAGMA " + name).Scan(&value).Error; err != nil {
		return 0, err
	}
	return value, nil
}

func isSafePragmaName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		isLetter := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
		if !isLetter && r != '_' {
			return false
		}
	}
	return true
}

// slowQueryThreshold — порог, после которого gorm логирует запрос как медленный.
// Для локальной SQLite с сотнями записей это уже признак проблемы.
const slowQueryThreshold = 200 * time.Millisecond

// gormWriter перекладывает вывод gorm в slog, чтобы в stdout был один формат логов.
type gormWriter struct {
	log *slog.Logger
}

func (w gormWriter) Printf(format string, args ...any) {
	w.log.Warn("gorm", "msg", strings.TrimSpace(fmt.Sprintf(format, args...)))
}
