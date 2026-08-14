// Package auth owns account and identity persistence primitives.
package auth

import "time"

// Tenant is a personal workspace that owns a user's resumes.
type Tenant struct {
	ID          int64     `db:"id" json:"id"`
	DisplayName string    `db:"display_name" json:"displayName"`
	CreatedAt   time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt   time.Time `db:"updated_at" json:"updatedAt"`
}

// User is an account with one default personal tenant.
type User struct {
	ID           int64     `db:"id" json:"id"`
	Email        string    `db:"email" json:"email"`
	PasswordHash string    `db:"password_hash" json:"-"`
	TenantID     int64     `db:"tenant_id" json:"tenantId"`
	IsAdmin      bool      `db:"is_admin" json:"isAdmin"`
	CreatedAt    time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time `db:"updated_at" json:"updatedAt"`
}

// Identity links a user to an authentication provider. Password is the first
// provider; OAuth providers such as WeChat can be added without changing user
// or tenant ownership.
type Identity struct {
	ID              int64     `db:"id" json:"id"`
	UserID          int64     `db:"user_id" json:"userId"`
	Provider        string    `db:"provider" json:"provider"`
	ProviderSubject string    `db:"provider_subject" json:"providerSubject"`
	CreatedAt       time.Time `db:"created_at" json:"createdAt"`
}

type CreateUserInput struct {
	Email    string
	Password string
}
