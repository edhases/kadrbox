package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/edhases/oxide-server/config"
	"github.com/edhases/oxide-server/internal/email"
	"github.com/edhases/oxide-server/internal/provider"
	"github.com/edhases/oxide-server/internal/repository/postgres"
	redisRepo "github.com/edhases/oxide-server/internal/repository/redis"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
	"github.com/edhases/oxide-server/internal/transport/ws"
)

func main() {
	cfg := config.Load()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	log.Printf("[Oxide Server] Starting on port %s...", cfg.ServerPort)

	// 1. Ініціалізація PostgreSQL з вбудованими міграціями (embed)
	dbPool, err := postgres.InitDB(ctx, cfg.PostgresDSN())
	if err != nil {
		log.Fatalf("[Postgres] Failed to initialize DB: %v", err)
	}
	defer dbPool.Close()

	// 2. Ініціалізація Redis
	redisClient, err := redisRepo.NewRedisClient(cfg.RedisAddr, cfg.RedisPass)
	if err != nil {
		log.Fatalf("[Redis] Failed to connect: %v", err)
	}
	defer redisClient.Close()
	log.Println("[Redis] Connected successfully")

	// 3. Репозиторії
	userRepo := postgres.NewUserRepository(dbPool)
	historyRepo := postgres.NewHistoryRepository(dbPool)
	favoritesRepo := postgres.NewFavoritesRepository(dbPool)
	cacheRepo := postgres.NewCacheRepository(dbPool)

	// 4. Фоновий воркер очищення кешу (кожні 6 годин)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[PANIC RECOVER] Cache worker panic: %v", rec)
			}
		}()
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				count, err := cacheRepo.DeleteExpired(ctx)
				if err == nil && count > 0 {
					log.Printf("[Cache Worker] Purged %d expired records from PostgreSQL", count)
				}
			}
		}
	}()

	// 5. Провайдери та емуляція браузерного TLS
	tlsClient, err := provider.NewTLSClient()
	if err != nil {
		log.Fatalf("[TLS Client] Failed to initialize: %v", err)
	}

	registry := provider.NewRegistry()
	registry.Register(provider.NewUakinoProvider(tlsClient))
	registry.Register(provider.NewEneyidaProvider(tlsClient))
	registry.Register(provider.NewHdrezkaProvider(tlsClient))
	registry.Register(provider.NewLavakinoProvider(tlsClient))
	registry.Register(provider.NewBanderaProvider())
	if disabled := cfg.GetDisabledProviders(); len(disabled) > 0 {
		registry.DisableMany(disabled)
		log.Printf("[Registry] Disabled providers (kill-switch): %v", disabled)
	}
	log.Printf("[Registry] Registered %d content providers", len(registry.List()))

	// 6. WebSocket Hub для Watch Party
	wsHub := ws.NewHub(redisClient)
	go wsHub.Run()

	// 7. HTTP Хендлери та Chi Роутер
	emailSvc := email.NewService()
	if emailSvc.IsConfigured() {
		log.Println("[Email] Resend service configured ✓")
	} else {
		log.Println("[Email] RESEND_API not set — email verification disabled (auto-verify mode)")
	}

	authHandler := transporthttp.NewAuthHandler(userRepo, redisClient, emailSvc, cfg.JWTSecret, cfg.GoogleClientID)
	authHandler.SetGoogleOAuth(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURI)
	authHandler.SetOAuth(
		cfg.TelegramBotToken,
		cfg.TelegramBotUsername,
		cfg.DiscordClientID,
		cfg.DiscordClientSecret,
		cfg.DiscordRedirectURI,
		cfg.AppURL,
	)
	if cfg.GoogleClientID != "" {
		log.Println("[OAuth] Google auth configured ✓")
	}
	if cfg.TelegramBotToken != "" {
		log.Println("[OAuth] Telegram auth configured ✓ (bot: @" + cfg.TelegramBotUsername + ")")
	}
	if cfg.DiscordClientID != "" {
		log.Println("[OAuth] Discord auth configured ✓")
	}

	contentHandler := transporthttp.NewContentHandler(registry, cacheRepo)
	syncHandler := transporthttp.NewSyncHandler(historyRepo, favoritesRepo)

	router := transporthttp.NewRouter(cfg.JWTSecret, authHandler, contentHandler, syncHandler, wsHub)

	server := &http.Server{
		Addr:         ":" + cfg.ServerPort,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Запуск HTTP сервера в окремій горутині
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[HTTP Server] ListenAndServe error: %v", err)
		}
	}()
	log.Printf("[Oxide Server] Ready and accepting connections on :%s", cfg.ServerPort)

	// 8. Graceful Shutdown (перехоплення SIGINT, SIGTERM)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[Oxide Server] Shutting down gracefully...")

	// 1. Припиняємо прийом нових HTTP/WS запитів (таймаут 10 секунд)
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("[HTTP Server] Shutdown error: %v", err)
	}

	// 2. Лише після закриття HTTP лістенера сповіщаємо та закриваємо клієнтів у WebSocket кімнатах
	wsHub.GracefulStop()

	log.Println("[Oxide Server] Server stopped cleanly")
}
