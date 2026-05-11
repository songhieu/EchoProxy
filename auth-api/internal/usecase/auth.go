package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"echoproxy/auth-api/internal/domain"
)

// Auth bundles signup, login and token issuance.
type Auth struct {
	users     domain.UserRepository
	jwtSecret []byte
	jwtTTL    time.Duration
}

func NewAuth(users domain.UserRepository, secret string, ttl time.Duration) *Auth {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Auth{users: users, jwtSecret: []byte(secret), jwtTTL: ttl}
}

func (a *Auth) Signup(ctx context.Context, email, password string) (*domain.User, string, error) {
	if !strings.Contains(email, "@") || len(password) < 8 {
		return nil, "", errors.New("invalid email or password too short")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", fmt.Errorf("hash: %w", err)
	}
	u, err := a.users.Create(ctx, strings.ToLower(email), string(hash))
	if err != nil {
		return nil, "", err
	}
	tok, err := a.issue(u)
	return u, tok, err
}

func (a *Auth) Login(ctx context.Context, email, password string) (*domain.User, string, error) {
	u, err := a.users.FindByEmail(ctx, strings.ToLower(email))
	if err != nil {
		return nil, "", domain.ErrInvalidCredentials
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)); err != nil {
		return nil, "", domain.ErrInvalidCredentials
	}
	tok, err := a.issue(u)
	return u, tok, err
}

func (a *Auth) Verify(token string) (uint64, error) {
	parsed, err := jwt.Parse(token, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return a.jwtSecret, nil
	})
	if err != nil || !parsed.Valid {
		return 0, fmt.Errorf("verify: %w", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return 0, errors.New("bad claims")
	}
	sub, ok := claims["sub"].(float64)
	if !ok {
		return 0, errors.New("bad sub")
	}
	return uint64(sub), nil
}

func (a *Auth) issue(u *domain.User) (string, error) {
	claims := jwt.MapClaims{
		"sub":   u.ID,
		"email": u.Email,
		"exp":   time.Now().Add(a.jwtTTL).Unix(),
		"iat":   time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(a.jwtSecret)
}

// GenerateAPIKey returns (rawKey, hash, prefix). The raw key is shown to the
// user once and never persisted.
func GenerateAPIKey() (raw, hash, prefix string, err error) {
	var buf [24]byte
	if _, err = rand.Read(buf[:]); err != nil {
		return "", "", "", err
	}
	raw = "sk_live_" + hex.EncodeToString(buf[:])
	prefix = raw[:12]
	sum := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(sum[:])
	return raw, hash, prefix, nil
}
