package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Ошибки работы с резервными копиями.
var (
	// ErrInvalidBackupName — имя не похоже на имя копии, созданной приложением.
	// Отдельная ошибка нужна, чтобы не пытаться читать произвольные пути.
	ErrInvalidBackupName = errors.New("недопустимое имя резервной копии")
	// ErrNotABackup — файл есть, но это не копия базы hopecore.
	ErrNotABackup = errors.New("файл не является резервной копией базы hopecore")
)

// backupNamePattern задаёт единственно допустимую форму имени.
//
// Это не косметика, а защита: имя приходит из HTTP-запроса и превращается
// в путь к файлу. Без строгой проверки «../../etc/passwd» дало бы чтение
// и удаление произвольных файлов.
var backupNamePattern = regexp.MustCompile(`^hopecore-\d{8}-\d{6}(-[a-z0-9-]+)?\.db$`)

// SuffixBeforeRestore помечает копию, снятую автоматически перед восстановлением.
// Благодаря ей ошибочное восстановление можно отменить.
const SuffixBeforeRestore = "before-restore"

// Backup — файл резервной копии.
type Backup struct {
	Name      string
	SizeBytes int64
	CreatedAt time.Time
	// Automatic — копия снята приложением сама, а не по кнопке.
	Automatic bool
}

// BackupDir возвращает каталог копий рядом с файлом базы.
//
// Копии лежат внутри того же bind-mount, поэтому видны с хоста и переживают
// пересборку контейнера.
func BackupDir(dbPath string) string {
	return filepath.Join(filepath.Dir(dbPath), "backups")
}

// CreateBackup снимает копию работающей базы.
//
// Используется VACUUM INTO: это единственный способ получить целостный снимок
// живой базы средствами SQL. Простое копирование файла может застать базу
// в середине записи, а с включённым журналом — потерять часть данных.
func CreateBackup(db *gorm.DB, dir, suffix string) (Backup, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Backup{}, fmt.Errorf("создание каталога копий: %w", err)
	}

	name, path, err := freeBackupPath(dir, time.Now().UTC(), suffix)
	if err != nil {
		return Backup{}, err
	}

	// VACUUM INTO отказывается писать в существующий файл, поэтому имя
	// подбирается заранее.
	if err := db.Exec(`VACUUM INTO ?`, path).Error; err != nil {
		return Backup{}, fmt.Errorf("снятие копии: %w", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		return Backup{}, fmt.Errorf("копия не найдена после создания: %w", err)
	}

	return Backup{
		Name:      name,
		SizeBytes: info.Size(),
		CreatedAt: info.ModTime(),
		Automatic: suffix != "",
	}, nil
}

// freeBackupPath подбирает свободное имя: две копии в одну секунду возможны.
func freeBackupPath(dir string, now time.Time, suffix string) (string, string, error) {
	base := "hopecore-" + now.Format("20060102-150405")
	if suffix != "" {
		base += "-" + suffix
	}

	for attempt := 0; attempt < 100; attempt++ {
		name := base + ".db"
		if attempt > 0 {
			name = fmt.Sprintf("%s-%d.db", base, attempt+1)
		}
		if !backupNamePattern.MatchString(name) {
			return "", "", fmt.Errorf("%w: %s", ErrInvalidBackupName, name)
		}

		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return name, path, nil
		}
	}

	return "", "", fmt.Errorf("не удалось подобрать имя для копии в %s", dir)
}

// ListBackups возвращает копии, свежие сверху.
func ListBackups(dir string) ([]Backup, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		// Каталога ещё нет — копий просто не делали.
		return []Backup{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("чтение каталога копий: %w", err)
	}

	backups := make([]Backup, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !backupNamePattern.MatchString(entry.Name()) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, Backup{
			Name:      entry.Name(),
			SizeBytes: info.Size(),
			CreatedAt: info.ModTime(),
			Automatic: strings.Contains(entry.Name(), SuffixBeforeRestore),
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		if backups[i].CreatedAt.Equal(backups[j].CreatedAt) {
			return backups[i].Name > backups[j].Name
		}
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	return backups, nil
}

// backupPath проверяет имя и собирает путь.
func backupPath(dir, name string) (string, error) {
	if !backupNamePattern.MatchString(name) {
		return "", fmt.Errorf("%w: %q", ErrInvalidBackupName, name)
	}

	path := filepath.Join(dir, name)

	// Двойная защита: даже если регулярное выражение когда-нибудь ослабят,
	// путь обязан остаться внутри каталога копий.
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if filepath.Dir(absPath) != absDir {
		return "", fmt.Errorf("%w: путь выходит за каталог копий", ErrInvalidBackupName)
	}

	if _, err := os.Stat(absPath); errors.Is(err, os.ErrNotExist) {
		return "", ErrNotFound
	} else if err != nil {
		return "", err
	}

	return absPath, nil
}

// DeleteBackup удаляет копию.
func DeleteBackup(dir, name string) error {
	path, err := backupPath(dir, name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("удаление копии: %w", err)
	}
	return nil
}

// RestoreBackup заменяет содержимое базы данными из копии.
//
// Файл базы не подменяется: приложение держит его открытым, и подмена на ходу
// означала бы гонку. Вместо этого копия подключается через ATTACH, и данные
// переносятся одной транзакцией — либо целиком, либо никак.
//
// Перед восстановлением автоматически снимается копия текущего состояния:
// ошибочное восстановление должно быть обратимым. Её имя возвращается вызывающему.
func RestoreBackup(db *gorm.DB, dir, name string) (Backup, error) {
	path, err := backupPath(dir, name)
	if err != nil {
		return Backup{}, err
	}

	safety, err := CreateBackup(db, dir, SuffixBeforeRestore)
	if err != nil {
		return Backup{}, fmt.Errorf("не удалось сохранить текущее состояние перед восстановлением: %w", err)
	}

	if err := db.Exec(`ATTACH DATABASE ? AS restore_src`, path).Error; err != nil {
		return safety, fmt.Errorf("%w: %v", ErrNotABackup, err)
	}
	defer func() {
		if err := db.Exec(`DETACH DATABASE restore_src`).Error; err != nil {
			// Отключить не удалось — соединение останется с лишней базой,
			// но данные уже целы.
			_ = err
		}
	}()

	var check string
	if err := db.Raw(`PRAGMA restore_src.integrity_check`).Scan(&check).Error; err != nil {
		return safety, fmt.Errorf("%w: %v", ErrNotABackup, err)
	}
	if !strings.EqualFold(check, "ok") {
		return safety, fmt.Errorf("%w: копия повреждена (%s)", ErrNotABackup, check)
	}

	source, err := tablesIn(db, "restore_src")
	if err != nil {
		return safety, err
	}
	// В копии должна быть хотя бы таблица вакансий, иначе это чужая база.
	if !source["vacancies"] {
		return safety, fmt.Errorf("%w: в копии нет таблицы vacancies", ErrNotABackup)
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		// Внешние ключи проверяются в конце транзакции, а не построчно:
		// иначе порядок переноса таблиц пришлось бы выстраивать вручную.
		if err := tx.Exec(`PRAGMA defer_foreign_keys = ON`).Error; err != nil {
			return err
		}

		for _, table := range TableNames() {
			if err := tx.Exec(`DELETE FROM main.` + quoteIdent(table)).Error; err != nil {
				return fmt.Errorf("очистка таблицы %s: %w", table, err)
			}
			// Таблицы, которой нет в копии, после восстановления быть не должно:
			// снимок сделан до её появления.
			if !source[table] {
				continue
			}
			sql := fmt.Sprintf(`INSERT INTO main.%[1]s SELECT * FROM restore_src.%[1]s`, quoteIdent(table))
			if err := tx.Exec(sql).Error; err != nil {
				return fmt.Errorf("перенос таблицы %s: %w", table, err)
			}
		}
		return nil
	})
	if err != nil {
		// Транзакция откатилась: база осталась в прежнем состоянии.
		return safety, fmt.Errorf("восстановление не выполнено, данные не изменены: %w", err)
	}

	return safety, nil
}

// tablesIn перечисляет таблицы подключённой базы.
func tablesIn(db *gorm.DB, schema string) (map[string]bool, error) {
	var names []string
	query := fmt.Sprintf(
		`SELECT name FROM %s.sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%%'`,
		quoteIdent(schema),
	)
	if err := db.Raw(query).Scan(&names).Error; err != nil {
		return nil, fmt.Errorf("чтение списка таблиц: %w", err)
	}

	tables := make(map[string]bool, len(names))
	for _, name := range names {
		tables[name] = true
	}
	return tables, nil
}

// quoteIdent обрамляет идентификатор кавычками. Имена таблиц берутся
// из TableNames() и не приходят от клиента, но подстановка в SQL без кавычек —
// плохая привычка.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
