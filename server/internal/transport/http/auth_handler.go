package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/edhases/oxide-server/internal/auth"
	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/repository/postgres"
	redisRepo "github.com/edhases/oxide-server/internal/repository/redis"
	"github.com/edhases/oxide-server/internal/transport/http/middleware"
)

type AuthHandler struct {
	userRepo    *postgres.UserRepository
	redisClient *redisRepo.RedisClient
	jwtSecret   string
}

func NewAuthHandler(userRepo *postgres.UserRepository, redisClient *redisRepo.RedisClient, jwtSecret string) *AuthHandler {
	return &AuthHandler{
		userRepo:    userRepo,
		redisClient: redisClient,
		jwtSecret:   jwtSecret,
	}
}

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

func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	if req.Email == "" || req.Password == "" || req.Username == "" {
		http.Error(w, `{"error":"email, password, and username are required"}`, http.StatusBadRequest)
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, `{"error":"failed to hash password"}`, http.StatusInternalServerError)
		return
	}

	user, err := h.userRepo.CreateUser(r.Context(), req.Email, hash, req.Username)
	if err != nil {
		http.Error(w, `{"error":"email already registered"}`, http.StatusConflict)
		return
	}

	h.respondWithTokens(w, r, user)
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	user, err := h.userRepo.GetUserByEmail(r.Context(), req.Email)
	if err != nil {
		http.Error(w, `{"error":"invalid email or password"}`, http.StatusUnauthorized)
		return
	}

	match, err := auth.ComparePasswordAndHash(req.Password, user.PasswordHash)
	if err != nil || !match {
		http.Error(w, `{"error":"invalid email or password"}`, http.StatusUnauthorized)
		return
	}

	h.respondWithTokens(w, r, user)
}

func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req RefreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request payload"}`, http.StatusBadRequest)
		return
	}

	userID, err := h.redisClient.GetUserIDByRefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		http.Error(w, `{"error":"invalid or expired refresh token"}`, http.StatusUnauthorized)
		return
	}

	user, err := h.userRepo.GetUserByID(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusUnauthorized)
		return
	}

	// Інвалідація старого refresh токена (rotation)
	_ = h.redisClient.RevokeRefreshToken(r.Context(), req.RefreshToken)

	h.respondWithTokens(w, r, user)
}

func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.GetUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	user, err := h.userRepo.GetUserByID(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

func (h *AuthHandler) respondWithTokens(w http.ResponseWriter, r *http.Request, user *domain.User) {
	accessToken, err := auth.GenerateAccessToken(user.ID, user.Email, user.Role, h.jwtSecret, 15*time.Minute)
	if err != nil {
		http.Error(w, `{"error":"failed to generate access token"}`, http.StatusInternalServerError)
		return
	}

	refreshToken := auth.GenerateRefreshToken()
	// Зберігаємо refresh токен у Redis на 30 днів
	if err := h.redisClient.StoreRefreshToken(r.Context(), refreshToken, user.ID, 30*24*time.Hour); err != nil {
		http.Error(w, `{"error":"failed to store session"}`, http.StatusInternalServerError)
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
