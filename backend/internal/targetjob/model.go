package targetjob

import "time"

type TargetJob struct {
	ID           int64     `db:"id" json:"id"`
	TenantID     int64     `db:"tenant_id" json:"-"`
	Company      string    `db:"company" json:"company"`
	Position     string    `db:"position" json:"position"`
	Requirements string    `db:"requirements" json:"requirements"`
	CreatedAt    time.Time `db:"created_at" json:"createdAt"`
	UpdatedAt    time.Time `db:"updated_at" json:"updatedAt"`
}

type CreateInput struct {
	Company      string
	Position     string
	Requirements string
}

type UpdateInput = CreateInput
