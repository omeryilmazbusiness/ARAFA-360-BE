package postgres

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/arafat-360/arafa-360-be/domain"
)

type ZoneRepository struct{ pool *pgxpool.Pool }

func NewZoneRepository(pool *pgxpool.Pool) *ZoneRepository { return &ZoneRepository{pool} }

var _ domain.ZoneRepository = (*ZoneRepository)(nil)

func (r *ZoneRepository) Create(ctx context.Context, z domain.Zone) error {
	_, err := r.pool.Exec(ctx,
		`INSERT INTO zones (id, tenant_id, name, coordinates_polygon, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		z.ID, z.TenantID, z.Name, z.CoordinatesPolygon, z.CreatedAt, z.UpdatedAt)
	return mapErr(err, "zone not found")
}
