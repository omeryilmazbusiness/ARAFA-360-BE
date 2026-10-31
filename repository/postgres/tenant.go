package postgres

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/arafat-360/arafa-360-be/domain"
)

type TenantRepository struct{ pool *pgxpool.Pool }

func NewTenantRepository(pool *pgxpool.Pool) *TenantRepository { return &TenantRepository{pool} }

var _ domain.TenantRepository = (*TenantRepository)(nil)

func (r *TenantRepository) GetByID(ctx context.Context, id uuid.UUID) (domain.Tenant, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, name, slug, status, created_at, updated_at FROM tenants WHERE id = $1`, id)
	if err != nil {
		return domain.Tenant{}, err
	}
	t, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[domain.Tenant])
	return t, mapErr(err, "tenant not found")
}
