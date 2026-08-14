package auth

import (
	"context"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const tokenIssuer = "resumepro"

var (
	ErrInvalidCredentials  = errors.New("invalid email or password")
	ErrInvalidRegistration = errors.New("invalid registration")
	ErrInvalidToken        = errors.New("invalid session token")
)

type Principal struct {
	UserID   int64
	TenantID int64
	Email    string
}

type Claims struct {
	UserID   int64  `json:"user_id"`
	TenantID int64  `json:"tenant_id"`
	Email    string `json:"email"`
	jwt.RegisteredClaims
}

type Service struct {
	repository *Repository
	secret     []byte
	now        func() time.Time
}

func NewService(repository *Repository, secret string) (*Service, error) {
	if repository == nil || strings.TrimSpace(secret) == "" {
		return nil, errors.New("authentication repository and JWT secret are required")
	}
	return &Service{repository: repository, secret: []byte(secret), now: time.Now}, nil
}

func (s *Service) Register(ctx context.Context, input CreateUserInput) (User, error) {
	if !validEmail(NormalizeEmail(input.Email)) || len(input.Password) < 12 {
		return User{}, ErrInvalidRegistration
	}
	return s.repository.CreateUser(ctx, input)
}

func (s *Service) Login(ctx context.Context, email, password string) (User, error) {
	if !validEmail(NormalizeEmail(email)) || password == "" {
		return User{}, ErrInvalidCredentials
	}
	user, err := s.repository.GetUserByEmail(ctx, email)
	if err != nil || !CheckPassword(user.PasswordHash, password) {
		return User{}, ErrInvalidCredentials
	}
	return user, nil
}

// FindUserByEmail returns a registered user for development-only session setup.
// Callers must enforce their own environment and request-origin restrictions.
func (s *Service) FindUserByEmail(ctx context.Context, email string) (User, error) {
	if !validEmail(NormalizeEmail(email)) {
		return User{}, ErrInvalidCredentials
	}
	return s.repository.GetUserByEmail(ctx, email)
}

// ValidEmail reports whether an input can identify a user account.
func ValidEmail(email string) bool {
	return validEmail(NormalizeEmail(email))
}

func (s *Service) IssueToken(user User) (string, error) {
	now := s.now().UTC()
	claims := Claims{
		UserID: user.ID, TenantID: user.TenantID, Email: user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: tokenIssuer, Subject: int64String(user.ID), IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(now.Add(7 * 24 * time.Hour)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
}

func (s *Service) VerifyToken(tokenString string) (Principal, error) {
	claims := Claims{}
	token, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, ErrInvalidToken
		}
		return s.secret, nil
	}, jwt.WithIssuer(tokenIssuer), jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !token.Valid || claims.UserID < 1 || claims.TenantID < 1 || claims.Subject != int64String(claims.UserID) || !validEmail(claims.Email) {
		return Principal{}, ErrInvalidToken
	}
	return Principal{UserID: claims.UserID, TenantID: claims.TenantID, Email: claims.Email}, nil
}

func validEmail(value string) bool {
	parsed, err := mail.ParseAddress(value)
	return err == nil && parsed.Address == NormalizeEmail(value) && strings.Contains(parsed.Address, "@")
}

func int64String(value int64) string {
	if value == 0 {
		return "0"
	}
	result := ""
	for value > 0 {
		result = string(rune('0'+value%10)) + result
		value /= 10
	}
	return result
}
