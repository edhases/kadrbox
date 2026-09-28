package http

import (
	"net/http"
	"net/url"
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
) *chi.Mux {
	r := chi.NewRouter()

	// 1. Базові middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)

	// 2. Безпечний CORS (дозволяємо нативні додатки без Origin, свій домен та локальні сервери)
	r.Use(cors.Handler(cors.Options{
		AllowOriginFunc: func(r *http.Request, origin string) bool {
			if origin == "" {
				return true
			}
			u, err := url.Parse(origin)
			if err != nil {
				return false
			}
			hostname := u.Hostname()
			return hostname == "localhost" ||
				hostname == "127.0.0.1" ||
				hostname == "oxideteam.pp.ua" ||
				strings.HasSuffix(hostname, ".oxideteam.pp.ua")
		},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Refresh-Token"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Healthcheck
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"oxide-server"}`))
	})

	// Веб-сторінки підтвердження email та скидання пароля при кліку з листа
	r.Get("/verify-email", authH.VerifyEmailWeb)
	r.Get("/reset-password", authH.ResetPasswordWeb)

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
		})

		// Публічний каталог і пошук
		r.Route("/content", func(r chi.Router) {
			r.Get("/search", contentH.Search)
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
			r.Delete("/auth/account", authH.DeleteAccount)

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

