package api

import (
	"errors"
	"net/http"
	"time"

	"github.com/Hundsamurai/hopecore/backend/internal/store"
)

// backupResponse — резервная копия в ответе API.
type backupResponse struct {
	Name      string    `json:"name"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
	// Automatic — копия снята приложением перед восстановлением, а не по кнопке.
	Automatic bool `json:"automatic"`
}

type backupsResponse struct {
	Items []backupResponse `json:"items"`
	// Dir — где лежат файлы. Полезно знать: копии можно забрать с диска руками.
	Dir string `json:"dir"`
	// TotalBytes — сколько места занято всеми копиями.
	TotalBytes int64 `json:"total_bytes"`
}

// restoreResponse сообщает, откуда восстановились и куда сохранили прежнее состояние.
type restoreResponse struct {
	Restored string `json:"restored"`
	// SafetyBackup — копия состояния до восстановления. Благодаря ей
	// ошибочное восстановление обратимо.
	SafetyBackup string `json:"safety_backup"`
}

func newBackupResponse(backup store.Backup) backupResponse {
	return backupResponse{
		Name:      backup.Name,
		SizeBytes: backup.SizeBytes,
		CreatedAt: backup.CreatedAt,
		Automatic: backup.Automatic,
	}
}

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	backups, err := store.ListBackups(s.backupDir)
	if err != nil {
		s.writeInternalError(w, r, err)
		return
	}

	items := make([]backupResponse, 0, len(backups))
	var total int64
	for _, backup := range backups {
		items = append(items, newBackupResponse(backup))
		total += backup.SizeBytes
	}

	writeJSON(w, http.StatusOK, backupsResponse{
		Items:      items,
		Dir:        s.backupDir,
		TotalBytes: total,
	})
}

func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	backup, err := store.CreateBackup(s.db, s.backupDir, "")
	if err != nil {
		s.writeInternalError(w, r, err)
		return
	}

	s.log.Info("создана резервная копия", "name", backup.Name, "size_bytes", backup.SizeBytes)
	writeJSON(w, http.StatusCreated, newBackupResponse(backup))
}

func (s *Server) handleRestoreBackup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	safety, err := store.RestoreBackup(s.db, s.backupDir, name)
	switch {
	case errors.Is(err, store.ErrInvalidBackupName), errors.Is(err, store.ErrNotABackup):
		// Имя пришло от клиента, поэтому недопустимое значение — это ошибка
		// запроса, а не сервера.
		writeError(w, http.StatusBadRequest, CodeValidationFailed, err.Error())
		return
	case errors.Is(err, store.ErrNotFound):
		writeNotFound(w, "резервная копия не найдена")
		return
	case err != nil:
		s.writeInternalError(w, r, err)
		return
	}

	s.log.Warn("база восстановлена из резервной копии",
		"restored", name, "safety_backup", safety.Name)

	writeJSON(w, http.StatusOK, restoreResponse{
		Restored:     name,
		SafetyBackup: safety.Name,
	})
}

func (s *Server) handleDeleteBackup(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	err := store.DeleteBackup(s.backupDir, name)
	switch {
	case errors.Is(err, store.ErrInvalidBackupName):
		writeError(w, http.StatusBadRequest, CodeValidationFailed, err.Error())
		return
	case errors.Is(err, store.ErrNotFound):
		writeNotFound(w, "резервная копия не найдена")
		return
	case err != nil:
		s.writeInternalError(w, r, err)
		return
	}

	s.log.Info("резервная копия удалена", "name", name)
	w.WriteHeader(http.StatusNoContent)
}
