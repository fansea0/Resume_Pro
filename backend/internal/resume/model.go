package resume

import "time"

// Resume is a persisted, structured resume document.
type Resume struct {
	ID         int64     `db:"id" json:"id"`
	TenantID   int64     `db:"tenant_id" json:"-"`
	Title      string    `db:"title" json:"title"`
	TemplateID string    `db:"template_id" json:"templateId"`
	Content    string    `db:"content" json:"content"`
	CreatedAt  time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt  time.Time `db:"updated_at" json:"updatedAt"`
}

type CreateInput struct {
	TenantID    int64
	AvatarOwner string
	Title       string
	TemplateID  string
	Content     string
}

type UpdateInput struct {
	AvatarOwner string
	Title       string
	TemplateID  string
	Content     string
}
