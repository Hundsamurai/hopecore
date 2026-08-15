// Команда server поднимает REST API трекера вакансий.
// Инструмент однопользовательский и локальный: авторизации нет,
// порт наружу пробрасывает только docker-compose на 127.0.0.1.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Hundsamurai/hopecore/backend/internal/activity"
	"github.com/Hundsamurai/hopecore/backend/internal/api"
	"github.com/Hundsamurai/hopecore/backend/internal/config"
	"github.com/Hundsamurai/hopecore/backend/internal/llm"
	"github.com/Hundsamurai/hopecore/backend/internal/service"
	"github.com/Hundsamurai/hopecore/backend/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		// Логгер ещё не настроен (уровень берётся из конфига), поэтому пишем напрямую.
		slog.Error("не удалось прочитать конфигурацию", "error", err)
		os.Exit(1)
	}

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	slog.SetDefault(log)

	if err := run(cfg, log); err != nil {
		log.Error("сервер остановлен с ошибкой", "error", err)
		os.Exit(1)
	}
}

func run(cfg config.Config, log *slog.Logger) error {
	db, err := store.Open(cfg.DBPath, log)
	if err != nil {
		return err
	}
	defer func() {
		if err := store.Close(db); err != nil {
			log.Error("не удалось закрыть БД", "error", err)
		}
	}()

	// Миграция на каждом старте: она идемпотентна, а отдельного шага
	// деплоя у локального инструмента нет.
	if err := store.Migrate(db); err != nil {
		return err
	}
	log.Info("схема БД актуальна", "path", cfg.DBPath, "tables", store.TableNames())

	checker := activity.NewHTTPChecker(cfg.CheckTimeout)
	activityService := service.NewActivityService(db, checker, cfg.CheckConcurrency, log)

	llmConfig, err := llm.LoadConfig()
	if err != nil {
		return err
	}
	logLLMConfig(log, llmConfig)

	deps := api.Deps{
		Log:      log,
		DB:       db,
		Activity: activityService,
		LLM:      llmConfig,
	}

	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           api.NewServer(deps).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		// Массовая проверка ходит по внешним сайтам и упирается в их скорость.
		// Ограничение сверху — bulkCheckTimeout в пакете api (4 минуты),
		// здесь запас, согласованный с proxy_read_timeout в nginx.
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	// Ошибка ListenAndServe уезжает в канал, чтобы main-горутина
	// могла одновременно ждать и падение сервера, и сигнал остановки.
	serveErr := make(chan error, 1)
	go func() {
		log.Info("сервер запущен", "addr", srv.Addr, "db_path", cfg.DBPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		log.Info("получен сигнал остановки, завершаем работу")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	return <-serveErr
}

// logLLMConfig сообщает при старте, какие провайдеры доступны.
// Ключи не логируются — только идентификаторы и модели.
func logLLMConfig(log *slog.Logger, cfg llm.Config) {
	available := cfg.Available()
	if len(available) == 0 {
		log.Info("провайдеры языковых моделей не настроены, заполнение через LLM недоступно")
		return
	}

	for _, provider := range available {
		log.Info("провайдер LLM доступен",
			"id", provider.ID,
			"models", provider.Models,
			"price_known", provider.Price.Known(),
		)
	}
}
