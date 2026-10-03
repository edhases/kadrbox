package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/edhases/oxide-server/config"
	"github.com/edhases/oxide-server/internal/email"
	"github.com/edhases/oxide-server/internal/logging"
	"github.com/edhases/oxide-server/internal/provider"
	"github.com/edhases/oxide-server/internal/repository/postgres"
	redisRepo "github.com/edhases/oxide-server/internal/repository/redis"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
	"github.com/edhases/oxide-server/internal/transport/http/middleware"
	"github.com/edhases/oxide-server/internal/transport/ws"
)

const (
	// shutdownTimeout is the total in-process drain budget. It must stay below the
	// orchestrator's grace period (docker-compose stop_grace_period): the hub drain
	// plus http.Server.Shutdown both draw from it, and at the old 10s the budget was
	// fully consumed before SIGKILL could arrive, so in-flight Watch Party publishes
	// were always cut off.
	shutdownTimeout = 20 * time.Second

	// Slowloris mitigation: bound how long a client may take to send request headers.
	readHeaderTimeout = 5 * time.Second

	// Cap total header bytes per request so header-flooding cannot exhaust memory.
	maxHeaderBytes = 1 << 20

	// cacheSweepInterval bounds how long an expired cache row lingers in
	// PostgreSQL. The shortest TTL written by the content handlers is 60s, so the
	// old 6-hour ticker let dead rows accumulate for 108x their lifetime. The
	// sweep itself is batched in the repository, so a short interval is cheap.
	cacheSweepInterval = 5 * time.Minute
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("[Config] refusing to start: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The first signal starts the graceful drain; a second one bails out immediately
	// so an operator can always break a hung drain (signal.NotifyContext alone would
	// silently swallow the repeat).
	quit := make(chan os.Signal, 2)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	go func() {
		if _, ok := <-quit; !ok {
			return
		}
		cancel()
		if _, ok := <-quit; ok {
			log.Println("[Oxide Server] Second signal received, forcing exit")
			os.Exit(130)
		}
	}()

	err := run(ctx, cfg)

	cancel()
	if err != nil {
		log.Printf("[Oxide Server] %v", err)
		// os.Exit is deliberately last: run() has already closed the DB and Redis
		// pools on return, and a non-zero code makes an incomplete drain observable
		// to Docker/orchestrators.
		os.Exit(1)
	}
}

// splitCSV розбиває значення environment-змінної типу "a, b ,c" на список.
// Порожні елементи відкидаються, щоб "a,,b" не створював порожнього allow-list.
func splitCSV(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func run(ctx context.Context, cfg *config.Config) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	log.Printf("[Oxide Server] Starting on port %s...", cfg.ServerPort)

	// Структуроване логування. Без цього middleware.RequestLogger та
	// logging.L() працюють через міст stdlib->slog у текстовому форматі.
	logging.Init(cfg.LogLevel, cfg.LogFormat)

	// 1. Ініціалізація PostgreSQL з вбудованими міграціями (embed)
	dbPool, err := postgres.InitDB(ctx, cfg.PostgresDSN())
	if err != nil {
		return fmt.Errorf("[Postgres] failed to initialize DB: %w", err)
	}
	defer dbPool.Close()

	// 2. Ініціалізація Redis
	redisClient, err := redisRepo.NewRedisClient(cfg.RedisAddr, cfg.RedisPass)
	if err != nil {
		return fmt.Errorf("[Redis] failed to connect: %w", err)
	}
	defer redisClient.Close()
	log.Println("[Redis] Connected successfully")

	// 3. Репозиторії
	userRepo := postgres.NewUserRepository(dbPool)
	historyRepo := postgres.NewHistoryRepository(dbPool)
	favoritesRepo := postgres.NewFavoritesRepository(dbPool)
	cacheRepo := postgres.NewCacheRepository(dbPool)

	// 4. Фоновий воркер очищення кешу (кожні 6 годин) + індексу сесій (щогодини)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[PANIC RECOVER] Cache worker panic: %v", rec)
			}
		}()
		cacheTicker := time.NewTicker(cacheSweepInterval)
		defer cacheTicker.Stop()
		sessionTicker := time.NewTicker(time.Hour)
		defer sessionTicker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-cacheTicker.C:
				count, err := cacheRepo.DeleteExpired(ctx)
				if err == nil && count > 0 {
					log.Printf("[Cache Worker] Purged %d expired records from PostgreSQL", count)
				}
			case <-sessionTicker.C:
				// Кожна ротація refresh-токена лишає один мертвий член у
				// sessions:<userID>, бо TTL індексу збігається з TTL токена.
				if pruned, err := redisClient.PruneExpiredSessions(ctx, 200); err == nil && pruned > 0 {
					log.Printf("[Session Worker] Pruned %d expired session index members", pruned)
				}
			}
		}
	}()

	// 5. Провайдери та емуляція браузерного TLS
	// SSRF allow-list має бути встановлений ДО створення провайдерів: без нього
	// ValidateSafeURL пропускає будь-який публічний хост, тобто ендпоінт
	// content/details перетворюється на проксі до довільних адрес. Список читається
	// з оточення (Portainer stack variable), порожнє значення = без обмежень.
	if hosts := splitCSV(os.Getenv("UPSTREAM_HOST_ALLOWLIST")); len(hosts) > 0 {
		transporthttp.SetUpstreamHostAllowlist(hosts)
		log.Printf("[SSRF] upstream host allow-list active (%d entries)", len(hosts))
	} else {
		log.Println("[SSRF] WARNING: UPSTREAM_HOST_ALLOWLIST is empty — any public host is reachable")
	}
	if sources := splitCSV(os.Getenv("UPSTREAM_SOURCE_ALLOWLIST")); len(sources) > 0 {
		transporthttp.SetUpstreamSourceAllowlist(sources)
	}
	if proxies := splitCSV(os.Getenv("TRUSTED_PROXY_CIDRS")); len(proxies) > 0 {
		if err := middleware.SetTrustedProxies(proxies); err != nil {
			return fmt.Errorf("[RateLimit] invalid TRUSTED_PROXY_CIDRS: %w", err)
		}
		log.Printf("[RateLimit] trusted proxies configured (%d entries)", len(proxies))
	}

	tlsClient, err := provider.NewTLSClient()
	if err != nil {
		return fmt.Errorf("[TLS Client] failed to initialize: %w", err)
	}

	registry := provider.NewRegistry()
	registry.Register(provider.NewUakinoProvider(tlsClient))
	registry.Register(provider.NewEneyidaProvider(tlsClient))
	registry.Register(provider.NewLavakinoProvider(tlsClient))
	registry.Register(provider.NewBanderaProvider())
	if disabled := cfg.GetDisabledProviders(); len(disabled) > 0 {
		registry.DisableMany(disabled)
		log.Printf("[Registry] Disabled providers (kill-switch): %v", disabled)
	}
	log.Printf("[Registry] Registered %d content providers", len(registry.List()))

	// 6. WebSocket Hub для Watch Party.
	// JWTSecret обов'язковий: без нього тікети не виписуються і хаб
	// fail-closed відповідає 503, а не приймає identity з query-параметрів.
	wsHub := ws.NewHub(redisClient, ws.Options{
		JWTSecret: cfg.JWTSecret,
		// Має збігатися з CORS allow-list у router.go, інакше браузер
		// проходить REST, але отримує 403 origin_not_allowed на WS handshake.
		AllowedOrigins:    transporthttp.AllowedOrigins(cfg.AppURL),
		MaxRooms:          ws.DefaultMaxRooms,
		MaxClientsPerRoom: ws.DefaultMaxClientsPerRoom,
		TicketTTL:         ws.DefaultTicketTTL,
	})
	go wsHub.Run()
	if err := wsHub.StartSubscription(ctx); err != nil {
		// Не фатально: без підписки фан-аут працює лише в межах одного інстансу.
		log.Printf("[WS Hub] cross-instance fan-out disabled: %v", err)
	}

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

	// Readiness-піни передаються до NewRouter, який знімає їх під час
	// ініціалізації. Без них /readyz відповідає 503 (fail-closed).
	middleware.SetPostgresPing(func(c context.Context) error { return dbPool.Ping(c) })
	middleware.SetRedisPing(func(c context.Context) error { return redisClient.Ping(c) })

	router := transporthttp.NewRouter(cfg.JWTSecret, authHandler, contentHandler, syncHandler, wsHub, cfg.AppURL)

	server := &http.Server{
		Addr:              ":" + cfg.ServerPort,
		Handler:           router,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: readHeaderTimeout,
		WriteTimeout:      45 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    maxHeaderBytes,
	}

	// Запуск HTTP сервера в окремій горутині. The error is reported through a
	// channel instead of log.Fatalf, which would skip the deferred pool cleanup.
	serveErr := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
			return
		}
		serveErr <- nil
	}()
	log.Printf("[Oxide Server] Ready and accepting connections on :%s", cfg.ServerPort)

	// 8. Graceful Shutdown (перехоплення SIGINT, SIGTERM)
	var shutdownErrs []error
	select {
	case <-ctx.Done():
		log.Println("[Oxide Server] Shutting down gracefully...")
	case err := <-serveErr:
		if err != nil {
			shutdownErrs = append(shutdownErrs, fmt.Errorf("[HTTP Server] ListenAndServe: %w", err))
			log.Printf("[Oxide Server] Shutting down after listener failure: %v", err)
		} else {
			log.Println("[Oxide Server] HTTP server stopped, shutting down...")
		}
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()

	// Order is deliberately hub-first, the reverse of the textbook http-first
	// pattern: WebSocket connections are hijacked out of net/http, so
	// server.Shutdown() closes the listener but never waits for them and would
	// return while pumps are still publishing to Redis. Stopping the hub first lets
	// those pumps drain before the listener goes away and before run() returns and
	// its defers close the Redis/Postgres pools.
	wsHub.GracefulStop()
	select {
	case <-wsHub.Done():
	case <-shutdownCtx.Done():
		shutdownErrs = append(shutdownErrs, errors.New("[WS Hub] hub loop did not exit within the drain budget"))
	}
	if err := wsHub.WaitPumps(shutdownCtx); err != nil {
		shutdownErrs = append(shutdownErrs, fmt.Errorf("[WS Hub] %w", err))
	}

	if err := server.Shutdown(shutdownCtx); err != nil {
		shutdownErrs = append(shutdownErrs, fmt.Errorf("[HTTP Server] shutdown: %w", err))
	}

	if err := errors.Join(shutdownErrs...); err != nil {
		return fmt.Errorf("shutdown did not complete cleanly: %w", err)
	}

	log.Println("[Oxide Server] Server stopped cleanly")
	return nil
}
