package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/arafat-360/arafa-360-be/domain"
)

type TentRepository struct{ pool *pgxpool.Pool }

func NewTentRepository(pool *pgxpool.Pool) *TentRepository { return &TentRepository{pool} }

var _ domain.TentRepository = (*TentRepository)(nil)

func (r *TentRepository) Create(ctx context.Context, t domain.Tent) error {
	// The composite FK (tenant_id, zone_id) rejects a zone from another tenant.
	_, err := r.pool.Exec(ctx,
		`INSERT INTO tents (id, tenant_id, zone_id, type, capacity, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		t.ID, t.TenantID, t.ZoneID, string(t.Type), t.Capacity, t.CreatedAt, t.UpdatedAt)
	return mapErr(err, "tent not found")
}

func (r *TentRepository) GetByID(ctx context.Context, tenantID, id uuid.UUID) (domain.Tent, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, tenant_id, zone_id, type, capacity, created_at, updated_at
		   FROM tents WHERE tenant_id = $1 AND id = $2`, tenantID, id)
	if err != nil {
		return domain.Tent{}, err
	}
	t, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[domain.Tent])
	return t, mapErr(err, "tent not found")
}
