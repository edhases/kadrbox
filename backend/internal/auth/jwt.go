package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var (
	ErrInvalidToken = errors.New("invalid or expired token")
	// ErrWrongIssuer / ErrWrongAudience are returned when a structurally valid
	// token was minted for a different consumer. Without them any service that
	// shares the signing secret would accept these tokens as its own.
	ErrWrongIssuer   = errors.New("token issuer mismatch")
	ErrWrongAudience = errors.New("token audience mismatch")
)

// Issuer and Audience pin what this service accepts. Tokens are only valid for
// this issuer and this audience; a token minted elsewhere with the same secret
// (a different service, a staging key reuse, a mis-issued link token) is
// rejected before any application logic runs.
const (
	Issuer   = "kadrbox-server"
	Audience = "oxide-api"
)

// KeyID is embedded in the `kid` header so a future multi-key deployment can
// tell which key signed a token without trial-decrypting with each secret.
const KeyID = "oxide-hs256-1"

type Claims struct {
	UserID uuid.UUID `json:"user_id"`
	Email  string    `json:"email"`
	Role   string    `json:"role"`
	jwt.RegisteredClaims
}

// GenerateAccessToken генерує короткоживучий JWT access-токен (15 хв)
func GenerateAccessToken(userID uuid.UUID, email, role, secret string, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Email:  email,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{Audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	token.Header["kid"] = KeyID
	return token.SignedString([]byte(secret))
}

// ValidateAccessToken перевіряє JWT токен і витягує Claims
//
// The parser is pinned to HS256 rather than "any HMAC": accepting HS512 too
// would let an attacker downgrade the signature the server verifies, and the
// algorithm is a property of the issuer, not of the token. Issuer and audience
// are asserted, so a token minted for another consumer of the same secret is
// not usable here.
func ValidateAccessToken(tokenString, secret string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(
		tokenString,
		claims,
		func(t *jwt.Token) (interface{}, error) {
			if t.Method != jwt.SigningMethodHS256 {
				return nil, ErrInvalidToken
			}
			return []byte(secret), nil
		},
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(Issuer),
		jwt.WithAudience(Audience),
		jwt.WithExpirationRequired(),
		jwt.WithLeeway(30*time.Second),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenInvalidIssuer) {
			return nil, ErrWrongIssuer
		}
		if errors.Is(err, jwt.ErrTokenInvalidAudience) {
			return nil, ErrWrongAudience
		}
		return nil, err
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// GenerateRefreshToken генерує криптографічно безпечний UUIDv4 для довготривалої сесії в Redis
func GenerateRefreshToken() string {
	return uuid.New().String()
}
