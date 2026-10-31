package domain

import (
	"time"

	"github.com/google/uuid"
)

type TenantStatus string

const (
	TenantActive    TenantStatus = "active"
	TenantSuspended TenantStatus = "suspended"
)

// Tenant is an agency: the isolation root. Every operational row in the
// system carries its tenant_id.
type Tenant struct {
	ID        uuid.UUID    `db:"id"         json:"id"`
	Name      string       `db:"name"       json:"name"`
	Slug      string       `db:"slug"       json:"slug"`
	Status    TenantStatus `db:"status"     json:"status"`
	CreatedAt time.Time    `db:"created_at" json:"created_at"`
	UpdatedAt time.Time    `db:"updated_at" json:"updated_at"`
}

func (t Tenant) Active() bool { return t.Status == TenantActive }
