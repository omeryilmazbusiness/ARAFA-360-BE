package domain

import (
	"crypto/rand"
	"encoding/hex"
	"regexp"
	"time"

	"github.com/google/uuid"
)

type BedStatus string

const (
	BedEmpty       BedStatus = "empty"
	BedOccupied    BedStatus = "occupied"
	BedMaintenance BedStatus = "maintenance"
)

func (s BedStatus) Valid() bool {
	switch s {
	case BedEmpty, BedOccupied, BedMaintenance:
		return true
	}
	return false
}

// Bed is the smallest addressable unit. Invariant (also enforced by a CHECK
// constraint): PilgrimID != nil  <=>  Status == BedOccupied.
type Bed struct {
	ID        uuid.UUID  `db:"id"         json:"id"`
	TenantID  uuid.UUID  `db:"tenant_id"  json:"tenant_id"`
	TentID    uuid.UUID  `db:"tent_id"    json:"tent_id"`
	PilgrimID *uuid.UUID `db:"pilgrim_id" json:"pilgrim_id,omitempty"`
	Status    BedStatus  `db:"status"     json:"status"`
	QRHash    string     `db:"qr_hash"    json:"qr_hash"`
	CreatedAt time.Time  `db:"created_at" json:"created_at"`
	UpdatedAt time.Time  `db:"updated_at" json:"updated_at"`
}

// NewBed creates an empty bed with a fresh, unguessable QR hash.
func NewBed(tenantID, tentID uuid.UUID) (Bed, error) {
	qr, err := NewQRHash()
	if err != nil {
		return Bed{}, err
	}
	now := time.Now().UTC()
	return Bed{
		ID:        uuid.New(),
		TenantID:  tenantID,
		TentID:    tentID,
		Status:    BedEmpty,
		QRHash:    qr,
		CreatedAt: now,
		UpdatedAt: now,
	}, nil
}

// NewQRHash returns 256 bits of CSPRNG output, hex encoded (64 chars). The
// value is the QR payload printed on the bed, so it must not be guessable.
func NewQRHash() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

var qrHashRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidQRHash rejects malformed payloads before they touch the database.
func ValidQRHash(s string) bool { return qrHashRe.MatchString(s) }
