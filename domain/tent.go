package domain

import (
	"time"

	"github.com/google/uuid"
)

type TentType string

const (
	TentStandard TentType = "standard"
	TentVIP      TentType = "vip"
	TentMedical  TentType = "medical"
	TentService  TentType = "service"
)

func (t TentType) Valid() bool {
	switch t {
	case TentStandard, TentVIP, TentMedical, TentService:
		return true
	}
	return false
}

const MaxTentCapacity = 5000

// Tent lives in a zone. TenantID is denormalised from the zone so every hot
// query can be filtered by tenant without a join; a composite foreign key
// (tenant_id, zone_id) -> zones(tenant_id, id) guarantees they never disagree.
type Tent struct {
	ID        uuid.UUID `db:"id"         json:"id"`
	TenantID  uuid.UUID `db:"tenant_id"  json:"tenant_id"`
	ZoneID    uuid.UUID `db:"zone_id"    json:"zone_id"`
	Type      TentType  `db:"type"       json:"type"`
	Capacity  int       `db:"capacity"   json:"capacity"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

func NewTent(tenantID, zoneID uuid.UUID, typ TentType, capacity int) (Tent, error) {
	if !typ.Valid() {
		return Tent{}, Invalid("type must be one of standard, vip, medical, service")
	}
	if capacity < 1 || capacity > MaxTentCapacity {
		return Tent{}, Invalid("capacity must be between 1 and %d", MaxTentCapacity)
	}
	now := time.Now().UTC()
	return Tent{
		ID:        uuid.New(),
		TenantID:  tenantID,
		ZoneID:    zoneID,
		Type:      typ,
		Capacity:  capacity,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}
