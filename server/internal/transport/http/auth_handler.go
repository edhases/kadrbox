package http

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/edhases/oxide-server/internal/auth"
	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/email"
	"github.com/edhases/oxide-server/internal/repository/postgres"
	redisRepo "github.com/edhases/oxide-server/internal/repository/redis"
	"github.com/edhases/oxide-server/internal/transport/http/middleware"
)

type AuthHandler struct {
	userRepo       *postgres.UserRepository
	redisClient    *redisRepo.RedisClient
	emailSvc       *email.Service
	jwtSecret      string
	googleClientID string
}

func NewAuthHandler(
	userRepo *postgres.UserRepository,
	redisClient *redisRepo.RedisClient,
	emailSvc *email.Service,
	jwtSecret string,
	googleClientID string,
) *AuthHandler {
	return &AuthHandler{
		userRepo:       userRepo,
		redisClient:    redisClient,
		emailSvc:       emailSvc,
		jwtSecret:      jwtSecret,
		googleClientID: googleClientID,
	}
}

// ---- request/response types -------------------------------------------------

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Username string `json:"username"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type AuthResponse struct {
	AccessToken  string       `json:"access_token"`
	RefreshToken string       `json:"refresh_token"`
	User         *domain.User `json:"user"`
}

// ---- handlers ---------------------------------------------------------------

// Register — POST /api/v1/auth/register
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request payload", http.StatusBadRequest)
		return
	}
	if req.Email == "" || req.Password == "" || req.Username == "" {
		jsonError(w, "email, password, and username are required", http.StatusBadRequest)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		jsonError(w, "failed to hash password", http.StatusInternalServerError)
		return
	}

	user, err := h.userRepo.CreateUser(r.Context(), req.Email, hash, req.Username)
	if err != nil {
		jsonError(w, "email already registered", http.StatusConflict)
		return
	}

	// Відправляємо лист підтвердження (якщо Resend налаштований)
	if h.emailSvc.IsConfigured() {
		token, tokenErr := generateToken()
		if tokenErr == nil {
			if storeErr := h.userRepo.CreateVerificationToken(r.Context(), user.ID, token); storeErr == nil {
				if sendErr := h.emailSvc.SendVerificationEmail(user.Email, user.Username, token); sendErr != nil {
					log.Printf("[Email] Failed to send verification to %s: %v", user.Email, sendErr)
				} else {
					log.Printf("[Email] Verification email sent to %s", user.Email)
				}
			}
		}
	} else {
		// RESEND_API не задано — автоматично підтверджуємо email
		log.Printf("[Email] RESEND_API not configured — auto-verifying %s", user.Email)
		_ = h.userRepo.MarkEmailVerified(r.Context(), user.ID)
		user.IsVerified = true
	}

	h.respondWithTokens(w, r, user)
}

// Login — POST /api/v1/auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	user, err := h.userRepo.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		jsonError(w, "invalid email or password", http.StatusUnauthorized)
		return
	}

	match, err := auth.ComparePasswordAndHash(req.Password, user.PasswordHash)
	if err != nil || !match {
		jsonError(w, "invalid email or password", http.StatusUnauthorized)
		return
	}

	// Якщо email не підтверджено і RESEND налаштовано — повертаємо 403
	if !user.IsVerified && h.emailSvc.IsConfigured() {
		jsonError(w, "email not verified", http.StatusForbidden)
		return
	}

	h.respondWithTokens(w, r, user)
}

// Refresh — POST /api/v1/auth/refresh
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	userID, err := h.redisClient.GetUserIDByRefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		jsonError(w, "invalid or expired refresh token", http.StatusUnauthorized)
		return
	}

	user, err := h.userRepo.GetUserByID(r.Context(), userID)
	if err != nil {
		jsonError(w, "user not found", http.StatusUnauthorized)
		return
	}

	// Ротація refresh токена
	_ = h.redisClient.RevokeRefreshToken(r.Context(), req.RefreshToken)

	h.respondWithTokens(w, r, user)
}

// Me — GET /api/v1/auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	user, err := h.userRepo.GetUserByID(r.Context(), userID)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

// VerifyEmail — POST /api/v1/auth/verify-email
// Body: {"token": "..."}
func (h *AuthHandler) VerifyEmail(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Token == "" {
		jsonError(w, "token is required", http.StatusBadRequest)
		return
	}

	userID, err := h.userRepo.GetUserByVerificationToken(r.Context(), body.Token)
	if err != nil {
		jsonError(w, "invalid or expired verification token", http.StatusBadRequest)
		return
	}

	if err := h.userRepo.MarkEmailVerified(r.Context(), userID); err != nil {
		jsonError(w, "failed to verify email", http.StatusInternalServerError)
		return
	}

	log.Printf("[Email] Email verified for user %s", userID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"email verified successfully"}`))
}

// ResendVerification — POST /api/v1/auth/resend-verification
// Body: {"email": "..."}
func (h *AuthHandler) ResendVerification(w http.ResponseWriter, r *http.Request) {
	if !h.emailSvc.IsConfigured() {
		jsonError(w, "email service not configured", http.StatusServiceUnavailable)
		return
	}

	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Email == "" {
		jsonError(w, "email is required", http.StatusBadRequest)
		return
	}

	user, err := h.userRepo.GetUserByEmail(r.Context(), body.Email)
	if err != nil {
		// Не розкриваємо що юзера не існує
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"if the email exists, a new verification link has been sent"}`))
		return
	}

	if user.IsVerified {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"email already verified"}`))
		return
	}

	token, err := generateToken()
	if err != nil {
		jsonError(w, "failed to generate token", http.StatusInternalServerError)
		return
	}

	if err := h.userRepo.CreateVerificationToken(r.Context(), user.ID, token); err != nil {
		jsonError(w, "failed to store token", http.StatusInternalServerError)
		return
	}

	if err := h.emailSvc.SendVerificationEmail(user.Email, user.Username, token); err != nil {
		log.Printf("[Email] Failed to resend verification to %s: %v", user.Email, err)
		jsonError(w, "failed to send email", http.StatusInternalServerError)
		return
	}

	log.Printf("[Email] Verification email resent to %s", user.Email)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"verification email sent"}`))
}

// UpdateProfile — PUT /api/v1/auth/profile
func (h *AuthHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Username  string `json:"username"`
		Name      string `json:"name"`
		Bio       string `json:"bio"`
		AvatarURL string `json:"avatar_url"`
		Avatar    string `json:"avatar"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, "invalid request payload", http.StatusBadRequest)
		return
	}

	username := req.Username
	if username == "" {
		username = req.Name
	}
	avatarURL := req.AvatarURL
	if avatarURL == "" {
		avatarURL = req.Avatar
	}

	user, err := h.userRepo.UpdateProfile(r.Context(), userID, username, req.Bio, avatarURL)
	if err != nil {
		jsonError(w, "failed to update profile", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

// ChangePassword — POST /api/v1/auth/change-password
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.OldPassword == "" || req.NewPassword == "" {
		jsonError(w, "old_password and new_password are required", http.StatusBadRequest)
		return
	}

	user, err := h.userRepo.GetUserByID(r.Context(), userID)
	if err != nil {
		jsonError(w, "user not found", http.StatusNotFound)
		return
	}

	match, err := auth.ComparePasswordAndHash(req.OldPassword, user.PasswordHash)
	if err != nil || !match {
		jsonError(w, "incorrect current password", http.StatusBadRequest)
		return
	}

	newHash, err := auth.HashPassword(req.NewPassword)
	if err != nil {
		jsonError(w, "failed to hash new password", http.StatusInternalServerError)
		return
	}

	if err := h.userRepo.UpdatePassword(r.Context(), userID, newHash); err != nil {
		jsonError(w, "failed to update password", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"password changed successfully"}`))
}

// DeleteAccount — DELETE /api/v1/auth/account
func (h *AuthHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := h.userRepo.DeleteUser(r.Context(), userID); err != nil {
		jsonError(w, "failed to delete user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"account deleted successfully"}`))
}

// ForgotPassword — POST /api/v1/auth/forgot-password
func (h *AuthHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" {
		jsonError(w, "email is required", http.StatusBadRequest)
		return
	}

	user, err := h.userRepo.GetUserByEmail(r.Context(), req.Email)
	if err == nil && h.emailSvc.IsConfigured() {
		if token, tokenErr := generateToken(); tokenErr == nil {
			if storeErr := h.userRepo.CreatePasswordResetToken(r.Context(), user.ID, token); storeErr == nil {
				if sendErr := h.emailSvc.SendPasswordResetEmail(user.Email, user.Username, token); sendErr != nil {
					log.Printf("[Email] Failed to send password reset email to %s: %v", user.Email, sendErr)
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"if the email exists, a password reset link has been sent"}`))
}

// ResetPassword — POST /api/v1/auth/reset-password
func (h *AuthHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" || req.Password == "" {
		jsonError(w, "token and password are required", http.StatusBadRequest)
		return
	}

	userID, err := h.userRepo.GetUserByPasswordResetToken(r.Context(), req.Token)
	if err != nil {
		jsonError(w, "invalid or expired reset token", http.StatusBadRequest)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		jsonError(w, "failed to hash password", http.StatusInternalServerError)
		return
	}

	if err := h.userRepo.UpdatePassword(r.Context(), userID, hash); err != nil {
		jsonError(w, "failed to update password", http.StatusInternalServerError)
		return
	}

	_ = h.userRepo.MarkPasswordResetUsed(r.Context(), userID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"message":"password reset successfully"}`))
}

// UploadAvatar — POST /api/v1/auth/avatar
func (h *AuthHandler) UploadAvatar(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		jsonError(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// 5 MB max
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		jsonError(w, "file too large (max 5MB)", http.StatusBadRequest)
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	file, header, err := r.FormFile("avatar")
	if err != nil {
		jsonError(w, "avatar file is required in 'avatar' field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".jpg" && ext != ".jpeg" && ext != ".png" && ext != ".webp" {
		jsonError(w, "only .jpg, .jpeg, .png, and .webp images are allowed", http.StatusBadRequest)
		return
	}

	// Валідація реального вмісту файлу (Magic Bytes / MIME-тип)
	buff := make([]byte, 512)
	n, readErr := file.Read(buff)
	if readErr != nil && readErr != io.EOF {
		jsonError(w, "failed to read file content", http.StatusBadRequest)
		return
	}
	contentType := http.DetectContentType(buff[:n])
	if !strings.HasPrefix(contentType, "image/jpeg") &&
		!strings.HasPrefix(contentType, "image/png") &&
		!strings.HasPrefix(contentType, "image/webp") {
		jsonError(w, "invalid image content type: "+contentType, http.StatusBadRequest)
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		jsonError(w, "failed to process file", http.StatusInternalServerError)
		return
	}

	uploadDir := "./data/uploads/avatars"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		jsonError(w, "failed to create upload directory", http.StatusInternalServerError)
		return
	}

	filename := fmt.Sprintf("%s%s", userID.String(), ext)
	targetPath := filepath.Join(uploadDir, filename)

	dst, err := os.Create(targetPath)
	if err != nil {
		jsonError(w, "failed to save avatar file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		jsonError(w, "failed to write avatar file", http.StatusInternalServerError)
		return
	}

	avatarURL := fmt.Sprintf("/uploads/avatars/%s?v=%d", filename, time.Now().Unix())

	user, err := h.userRepo.UpdateProfile(r.Context(), userID, "", "", avatarURL)
	if err != nil {
		jsonError(w, "failed to update user avatar in database", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

// GoogleAuth — POST /api/v1/auth/google
func (h *AuthHandler) GoogleAuth(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDToken string `json:"id_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IDToken == "" {
		jsonError(w, "id_token is required", http.StatusBadRequest)
		return
	}

	// Валідація ID Token через Google TokenInfo API з таймаутом
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	tokenURL := fmt.Sprintf("https://oauth2.googleapis.com/tokeninfo?id_token=%s", url.QueryEscape(req.IDToken))
	reqHttp, err := http.NewRequestWithContext(ctx, http.MethodGet, tokenURL, nil)
	if err != nil {
		jsonError(w, "failed to build google request", http.StatusInternalServerError)
		return
	}

	resp, err := http.DefaultClient.Do(reqHttp)
	if err != nil {
		jsonError(w, "failed to contact google oauth", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		jsonError(w, "invalid google id_token", http.StatusUnauthorized)
		return
	}

	var info struct {
		Aud           string `json:"aud"`
		Email         string `json:"email"`
		EmailVerified string `json:"email_verified"`
		Name          string `json:"name"`
		Picture       string `json:"picture"`
		Sub           string `json:"sub"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil || info.Email == "" {
		jsonError(w, "invalid google token payload", http.StatusUnauthorized)
		return
	}

	// Перевірка статусу верифікації пошти в Google
	if info.EmailVerified != "true" && info.EmailVerified != "1" {
		jsonError(w, "google email is not verified", http.StatusUnauthorized)
		return
	}

	// Перевірка Audience (Client ID) якщо налаштовано на сервері
	if h.googleClientID != "" && info.Aud != h.googleClientID {
		jsonError(w, "google token audience mismatch", http.StatusUnauthorized)
		return
	}

	// Шукаємо користувача за email
	user, err := h.userRepo.GetUserByEmail(r.Context(), info.Email)
	if err != nil {
		if errors.Is(err, postgres.ErrUserNotFound) {
			// Створюємо нового користувача
			randomPass, _ := generateToken()
			hash, hashErr := auth.HashPassword(randomPass)
			if hashErr != nil {
				jsonError(w, "failed to hash password", http.StatusInternalServerError)
				return
			}

			username := info.Name
			if username == "" {
				username = strings.Split(info.Email, "@")[0]
			}

			newUser, createErr := h.userRepo.CreateUser(r.Context(), info.Email, hash, username)
			if createErr != nil {
				jsonError(w, "failed to create user", http.StatusInternalServerError)
				return
			}

			// Google email вже перевірений
			_ = h.userRepo.MarkEmailVerified(r.Context(), newUser.ID)
			newUser.IsVerified = true

			if info.Picture != "" {
				updatedUser, updateErr := h.userRepo.UpdateProfile(r.Context(), newUser.ID, "", "", info.Picture)
				if updateErr == nil {
					newUser = updatedUser
				}
			}
			user = newUser
		} else {
			jsonError(w, "database error", http.StatusInternalServerError)
			return
		}
	} else {
		// Користувач існує: верифікуємо email через Google якщо ще не був
		if !user.IsVerified {
			_ = h.userRepo.MarkEmailVerified(r.Context(), user.ID)
			user.IsVerified = true
		}
		if user.AvatarURL == "" && info.Picture != "" {
			updatedUser, _ := h.userRepo.UpdateProfile(r.Context(), user.ID, "", "", info.Picture)
			if updatedUser != nil {
				user = updatedUser
			}
		}
	}

	h.respondWithTokens(w, r, user)
}

// ---- helpers ----------------------------------------------------------------

func (h *AuthHandler) respondWithTokens(w http.ResponseWriter, r *http.Request, user *domain.User) {
	accessToken, err := auth.GenerateAccessToken(user.ID, user.Email, user.Role, h.jwtSecret, 15*time.Minute)
	if err != nil {
		jsonError(w, "failed to generate access token", http.StatusInternalServerError)
		return
	}

	refreshToken := auth.GenerateRefreshToken()
	if err := h.redisClient.StoreRefreshToken(r.Context(), refreshToken, user.ID, 30*24*time.Hour); err != nil {
		jsonError(w, "failed to store session", http.StatusInternalServerError)
		return
	}

	resp := AuthResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		User:         user,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func generateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// VerifyEmailWeb — GET /verify-email?token=...
func (h *AuthHandler) VerifyEmailWeb(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if token == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(renderEmailStatusHTML(false, "Токен відсутній", "Посилання не містить токена підтвердження.")))
		return
	}

	userID, err := h.userRepo.GetUserByVerificationToken(r.Context(), token)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(renderEmailStatusHTML(false, "Посилання недійсне або застаріло", "Термін дії посилання закінчився (24 години) або воно вже було використане.")))
		return
	}

	if err := h.userRepo.MarkEmailVerified(r.Context(), userID); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(renderEmailStatusHTML(false, "Помилка сервера", "Не вдалося зберегти підтвердження email. Спробуйте пізніше.")))
		return
	}

	log.Printf("[Email] Web verification successful for user %s", userID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(renderEmailStatusHTML(true, "Пошту успішно підтверджено!", "Ваш акаунт активовано. Тепер ви можете увійти у застосунок Oxide Film.")))
}

// ResetPasswordWeb — GET /reset-password?token=...
func (h *AuthHandler) ResetPasswordWeb(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if token == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(renderEmailStatusHTML(false, "Токен відсутній", "Посилання не містить токена скидання пароля.")))
		return
	}

	_, err := h.userRepo.GetUserByPasswordResetToken(r.Context(), token)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(renderEmailStatusHTML(false, "Посилання недійсне або застаріло", "Термін дії посилання для скидання пароля минув (1 година) або воно вже використане.")))
		return
	}

	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="uk">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>Скидання пароля — Oxide Film</title>
  <style>
    body {
      margin: 0; padding: 0;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      background: #0f172a; color: #f8fafc;
      display: flex; align-items: center; justify-content: center; min-height: 100vh;
    }
    .card {
      background: #1e293b; border: 1px solid #334155; border-radius: 16px;
      padding: 36px; max-width: 400px; width: 90%%; box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5);
    }
    .logo { color: #6366f1; font-weight: 700; text-transform: uppercase; font-size: 14px; margin-bottom: 8px; }
    h1 { font-size: 20px; margin: 0 0 16px; color: #fff; }
    p { color: #94a3b8; font-size: 14px; margin: 0 0 20px; line-height: 1.5; }
    input {
      width: 100%%; padding: 12px; border-radius: 8px; border: 1px solid #475569;
      background: #0f172a; color: #fff; font-size: 15px; margin-bottom: 16px; box-sizing: border-box;
    }
    input:focus { outline: none; border-color: #6366f1; }
    button {
      width: 100%%; padding: 12px; background: #6366f1; color: #fff; border: none;
      border-radius: 8px; font-size: 15px; font-weight: bold; cursor: pointer;
    }
    button:hover { background: #4f46e5; }
    .msg { margin-top: 16px; font-size: 14px; display: none; }
  </style>
</head>
<body>
  <div class="card">
    <div class="logo">Oxide Film</div>
    <h1>Новий пароль</h1>
    <p>Введіть новий пароль для вашого акаунту (мінімум 6 символів):</p>
    <input type="password" id="pwd" placeholder="Новий пароль" minlength="6" required />
    <button onclick="submitReset()">Зберегти пароль</button>
    <div id="res" class="msg"></div>
  </div>
  <script>
    async function submitReset() {
      const p = document.getElementById('pwd').value;
      const res = document.getElementById('res');
      if (!p || p.length < 6) {
        res.style.display = 'block'; res.style.color = '#ef4444';
        res.innerText = 'Пароль має містити щонайменше 6 символів';
        return;
      }
      try {
        const resp = await fetch('/api/v1/auth/reset-password', {
          method: 'POST',
          headers: {'Content-Type': 'application/json'},
          body: JSON.stringify({token: %q, password: p})
        });
        const data = await resp.json();
        res.style.display = 'block';
        if (resp.ok) {
          res.style.color = '#10b981';
          res.innerText = 'Пароль успішно змінено! Тепер ви можете увійти в застосунок.';
          document.getElementById('pwd').style.display = 'none';
          document.querySelector('button').style.display = 'none';
        } else {
          res.style.color = '#ef4444';
          res.innerText = data.error || 'Помилка при збереженні пароля';
        }
      } catch (e) {
        res.style.display = 'block'; res.style.color = '#ef4444';
        res.innerText = 'Мережева помилка';
      }
    }
  </script>
</body>
</html>`, token)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

func renderEmailStatusHTML(success bool, title, message string) string {
	icon := "✅"
	accentColor := "#6366f1"
	if !success {
		icon = "❌"
		accentColor = "#ef4444"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="uk">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>%s — Oxide Film</title>
  <style>
    body {
      margin: 0; padding: 0;
      font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
      background: #0f172a; color: #f8fafc;
      display: flex; align-items: center; justify-content: center; min-height: 100vh;
    }
    .card {
      background: #1e293b; border: 1px solid #334155; border-radius: 16px;
      padding: 40px; max-width: 440px; margin: 20px; text-align: center;
      box-shadow: 0 20px 25px -5px rgba(0, 0, 0, 0.5);
    }
    .icon { font-size: 54px; margin-bottom: 20px; }
    h1 { font-size: 22px; margin: 0 0 12px; color: #fff; }
    p { color: #94a3b8; font-size: 15px; line-height: 1.5; margin: 0 0 24px; }
    .logo { font-size: 14px; color: %s; font-weight: 700; letter-spacing: 0.5px; text-transform: uppercase; margin-bottom: 8px; }
  </style>
</head>
<body>
  <div class="card">
    <div class="logo">Oxide Film</div>
    <div class="icon">%s</div>
    <h1>%s</h1>
    <p>%s</p>
  </div>
</body>
</html>`, title, accentColor, icon, title, message)
}
