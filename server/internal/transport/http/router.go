package http

import (
	"net/http"
	"strings"

	"github.com/edhases/oxide-server/internal/transport/http/middleware"
	"github.com/edhases/oxide-server/internal/transport/ws"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

func NewRouter(
	jwtSecret string,
	authH *AuthHandler,
	contentH *ContentHandler,
	syncH *SyncHandler,
	hub *ws.Hub,
	appURL string,
) *chi.Mux {
	r := chi.NewRouter()

	// 1. Базові middleware
	// Порядок важливий: RequestID -> RequestContext (trace id) -> RequestLogger
	// (пише trace_id у лог) -> CORS -> Recoverer (логер бачить 500 після паніки)
	// -> BodyLimit -> RateLimit.
	//
	// chimiddleware.RealIP НЕ підключено свідомо: він переписує r.RemoteAddr
	// значенням із X-Real-IP/X-Forwarded-For, знищуючи справжню адресу піра.
	// Через це атакувач міг підробляти заголовок і отримувати новий rate-limit
	// бакет на кожен запит. middleware.RateLimitMiddleware сам коректно
	// проходить ланцюг X-Forwarded-For справа наліво, враховуючи
	// TRUSTED_PROXY_CIDRS, і використовує RemoteAddr за замовчуванням.
	r.Use(chimiddleware.RequestID)
	r.Use(middleware.RequestContext)
	r.Use(middleware.RequestLogger)

	// 2. Безпечний CORS (дозволяємо нативні додатки без Origin, свій домен та локальні сервери)
	// Той самий allow-list передається в ws.Options, інакше браузер проходить
	// REST, але отримує 403 origin_not_allowed на WebSocket handshake.
	allowedOrigins := AllowedOrigins(appURL)
	r.Use(cors.Handler(cors.Options{
		AllowOriginFunc: func(r *http.Request, origin string) bool {
			return originAllowed(origin, allowedOrigins)
		},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Refresh-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(chimiddleware.Recoverer)

	// Глобальна стеля розміру тіла запиту (1 MiB) — до будь-якого хендлера.
	r.Use(middleware.BodyLimit(middleware.DefaultMaxBodyBytes))
	r.Use(middleware.RateLimitMiddleware(middleware.NewIPRateLimiter(30, 60)))

	// Healthcheck: liveness не торкається залежностей, readiness — так.
	// Пінги передаються з main.go через middleware.SetPostgresPing/SetRedisPing;
	// без них /readyz навмисно відповідає 503 (fail-closed).
	healthH := middleware.New(middleware.RedisPing(), "oxide-server")

	// /health — історичний шлях, збережений як alias до /healthz
	r.Get("/health", healthH.Livez)
	r.Get("/healthz", healthH.Livez)
	r.Get("/readyz", healthH.Readyz)

	// Веб-сторінки підтвердження email та скидання пароля при кліку з листа
	r.Get("/verify-email", authH.VerifyEmailWeb)
	r.Get("/reset-password", authH.ResetPasswordWeb)
	r.Get("/auth/telegram", authH.TelegramLoginWeb)
	r.Get("/auth/discord", authH.DiscordLogin)
	r.Get("/auth/google", authH.GoogleLogin)

	// WebSocket Watch Party
	r.Get("/api/v1/ws/watch-party", hub.HandleWebSocket)

	// Роздача завантажених файлів (аватари тощо) без Directory Listing (BUG-GO-06)
	r.Handle("/uploads/*", http.StripPrefix("/uploads/", fileServerNoListing("./data/uploads")))

	// REST API v1
	r.Route("/api/v1", func(r chi.Router) {
		// Публічні ендпоінти авторизації
		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", authH.Register)
			r.Post("/login", authH.Login)
			r.Post("/refresh", authH.Refresh)
			r.Post("/verify-email", authH.VerifyEmail)
			r.Post("/resend-verification", authH.ResendVerification)
			r.Post("/forgot-password", authH.ForgotPassword)
			r.Post("/reset-password", authH.ResetPassword)
			r.Post("/google", authH.GoogleAuth)
			r.Get("/google/login", authH.GoogleLogin)
			r.Get("/google/callback", authH.GoogleCallback)
			r.Post("/telegram", authH.TelegramAuth)
			r.Get("/telegram/login", authH.TelegramLoginWeb)
			r.Get("/telegram/callback", authH.TelegramCallbackWeb)
			r.Post("/discord", authH.DiscordAuthAPI)
			r.Get("/discord/login", authH.DiscordLogin)
			r.Get("/discord/callback", authH.DiscordCallback)
		})

		// Публічний каталог і пошук
		r.Route("/content", func(r chi.Router) {
			r.Get("/providers", contentH.Providers)
			r.Get("/search", contentH.Search)
			r.Get("/popular", contentH.Popular)
			r.Get("/category", contentH.Category)
			r.Get("/details", contentH.GetDetails)
			r.Get("/streams", contentH.GetStreams)
		})

		// Захищені ендпоінти користувача
		r.Group(func(r chi.Router) {
			r.Use(middleware.AuthMiddleware(jwtSecret))

			r.Get("/auth/me", authH.Me)
			r.Put("/auth/profile", authH.UpdateProfile)
			r.Post("/auth/avatar", authH.UploadAvatar)
			r.Post("/auth/change-password", authH.ChangePassword)
			r.Post("/auth/unlink", authH.UnlinkProvider)
			r.Post("/auth/logout", authH.Logout)
			r.Delete("/auth/account", authH.DeleteAccount)

			// Квитки до Watch Party: identity більше не приймається з query,
			// клієнт спочатку отримує короткочасний тікет через цей ендпоінт.
			r.Post("/watch-party/tickets", NewWatchPartyHandler(hub).Issue)

			r.Group(func(r chi.Router) {
				// Синхронізація — цінність акаунту: тільки для підтверджених пошт
				r.Use(authH.RequireVerifiedEmail())
				r.Route("/sync", func(r chi.Router) {
					r.Get("/history", syncH.GetHistory)
					r.Post("/history", syncH.SaveProgress)
					r.Get("/continue-watching", syncH.GetContinueWatching)
					r.Get("/favorites", syncH.GetFavorites)
					r.Post("/favorites/toggle", syncH.ToggleFavorite)
					r.Delete("/favorites", syncH.RemoveFavorite)
				})
			})
		})
	})

	return r
}

// fileServerNoListing запобігає виводу Directory Listing для папок
func fileServerNoListing(root string) http.Handler {
	fs := http.Dir(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/") {
			http.NotFound(w, r)
			return
		}
		f, err := fs.Open(r.URL.Path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			http.NotFound(w, r)
			return
		}
		http.FileServer(fs).ServeHTTP(w, r)
	})
}
