package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmailAlreadyRegistered = errors.New("email already registered")
	ErrUserNotFound           = errors.New("user not found")
)

const passwordHashCost = 12

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// NormalizeEmail is shared by registration and password identity lookup.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), passwordHashCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// CreateUser creates an account, its private tenant, and its password identity
// atomically. Email is normalized before both uniqueness checks and storage.
func (r *Repository) CreateUser(ctx context.Context, input CreateUserInput) (User, error) {
	email := NormalizeEmail(input.Email)
	hash, err := HashPassword(input.Password)
	if err != nil {
		return User{}, err
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UTC()
	tenantResult, err := tx.ExecContext(ctx, `INSERT INTO tenants (display_name, created_at, updated_at) VALUES (?, ?, ?)`, "Personal workspace", now, now)
	if err != nil {
		return User{}, err
	}
	tenantID, err := tenantResult.LastInsertId()
	if err != nil {
		return User{}, err
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO users (email, password_hash, tenant_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, email, hash, tenantID, now, now)
	if err != nil {
		if isUniqueConstraint(err) {
			return User{}, ErrEmailAlreadyRegistered
		}
		return User{}, err
	}
	userID, err := result.LastInsertId()
	if err != nil {
		return User{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO user_identities (user_id, provider, provider_subject, created_at) VALUES (?, 'password', ?, ?)`, userID, email, now); err != nil {
		return User{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, err
	}
	return User{ID: userID, Email: email, PasswordHash: hash, TenantID: tenantID, IsAdmin: false, CreatedAt: now, UpdatedAt: now}, nil
}

// SetAdministrator changes one persisted user's administrator role.
func (r *Repository) SetAdministrator(ctx context.Context, userID int64, isAdmin bool) error {
	result, err := r.db.ExecContext(ctx, `UPDATE users SET is_admin = ? WHERE id = ?`, isAdmin, userID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrUserNotFound
	}
	return nil
}

func (r *Repository) GetUserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := r.db.GetContext(ctx, &user, `SELECT id, email, password_hash, tenant_id, is_admin, created_at, updated_at FROM users WHERE email = ?`, NormalizeEmail(email))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	return user, err
}

func (r *Repository) GetUserByID(ctx context.Context, id int64) (User, error) {
	var user User
	err := r.db.GetContext(ctx, &user, `SELECT id, email, password_hash, tenant_id, is_admin, created_at, updated_at FROM users WHERE id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrUserNotFound
	}
	return user, err
}

func isUniqueConstraint(err error) bool {
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint")
}
