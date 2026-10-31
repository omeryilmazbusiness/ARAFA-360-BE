package usecase

import (
	"context"

	"github.com/google/uuid"

	"github.com/arafat-360/arafa-360-be/domain"
)

// MaxBedsPerRequest bounds a single provisioning call.
const MaxBedsPerRequest = 500

// InventoryUsecase manages the physical layout: zones, tents and beds.
type InventoryUsecase struct {
	zones domain.ZoneRepository
	tents domain.TentRepository
	beds  domain.BedRepository
}

func NewInventoryUsecase(z domain.ZoneRepository, t domain.TentRepository, b domain.BedRepository) *InventoryUsecase {
	return &InventoryUsecase{zones: z, tents: t, beds: b}
}

func (u *InventoryUsecase) CreateZone(ctx context.Context, tenantID uuid.UUID, name string, polygon domain.Polygon) (domain.Zone, error) {
	z, err := domain.NewZone(tenantID, name, polygon)
	if err != nil {
		return domain.Zone{}, err
	}
	return z, u.zones.Create(ctx, z)
}

func (u *InventoryUsecase) CreateTent(ctx context.Context, tenantID, zoneID uuid.UUID, typ domain.TentType, capacity int) (domain.Tent, error) {
	t, err := domain.NewTent(tenantID, zoneID, typ, capacity)
	if err != nil {
		return domain.Tent{}, err
	}
	return t, u.tents.Create(ctx, t)
}

// ProvisionBeds creates `count` empty beds (each with its own QR hash) in a tent.
func (u *InventoryUsecase) ProvisionBeds(ctx context.Context, tenantID, tentID uuid.UUID, count int) ([]domain.Bed, error) {
	if count < 1 || count > MaxBedsPerRequest {
		return nil, domain.Invalid("count must be between 1 and %d", MaxBedsPerRequest)
	}
	return u.beds.CreateForTent(ctx, tenantID, tentID, count)
}
