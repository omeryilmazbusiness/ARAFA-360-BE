package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/arafat-360/arafa-360-be/domain"
)

type BedRepository struct{ pool *pgxpool.Pool }

func NewBedRepository(pool *pgxpool.Pool) *BedRepository { return &BedRepository{pool} }

var _ domain.BedRepository = (*BedRepository)(nil)

const bedColumns = `id, tenant_id, tent_id, pilgrim_id, status, qr_hash, created_at, updated_at`

func (r *BedRepository) GetByQRHash(ctx context.Context, tenantID uuid.UUID, qrHash string) (domain.Bed, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+bedColumns+` FROM beds WHERE tenant_id = $1 AND qr_hash = $2`, tenantID, qrHash)
	if err != nil {
		return domain.Bed{}, err
	}
	b, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[domain.Bed])
	return b, mapErr(err, "bed not found")
}

// CheckIn is a single compare-and-set statement: under thousands of
// concurrent scans exactly one wins, with no explicit transaction or lock.
func (r *BedRepository) CheckIn(ctx context.Context, tenantID uuid.UUID, qrHash string, pilgrimID uuid.UUID) (domain.Bed, error) {
	rows, err := r.pool.Query(ctx,
		`UPDATE beds
		    SET pilgrim_id = $3, status = 'occupied', updated_at = now()
		  WHERE tenant_id = $1 AND qr_hash = $2 AND status = 'empty'
		  RETURNING `+bedColumns, tenantID, qrHash, pilgrimID)
	if err != nil {
		return domain.Bed{}, err
	}
	b, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[domain.Bed])
	if err == nil {
		return b, nil
	}
	if !isNoRows(err) {
		return domain.Bed{}, mapErr(err, "bed not found")
	}
	return domain.Bed{}, r.explainRejection(ctx, tenantID, qrHash, "cannot check in")
}

func (r *BedRepository) CheckOut(ctx context.Context, tenantID uuid.UUID, qrHash string) (domain.Bed, error) {
	rows, err := r.pool.Query(ctx,
		`UPDATE beds
		    SET pilgrim_id = NULL, status = 'empty', updated_at = now()
		  WHERE tenant_id = $1 AND qr_hash = $2 AND status = 'occupied'
		  RETURNING `+bedColumns, tenantID, qrHash)
	if err != nil {
		return domain.Bed{}, err
	}
	b, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[domain.Bed])
	if err == nil {
		return b, nil
	}
	if !isNoRows(err) {
		return domain.Bed{}, mapErr(err, "bed not found")
	}
	return domain.Bed{}, r.explainRejection(ctx, tenantID, qrHash, "cannot check out")
}

// explainRejection runs only on the (rare) failure path to tell "no such bed"
// apart from "bed is in the wrong state".
func (r *BedRepository) explainRejection(ctx context.Context, tenantID uuid.UUID, qrHash, action string) error {
	b, err := r.GetByQRHash(ctx, tenantID, qrHash)
	if err != nil {
		return err
	}
	return domain.Conflict("%s: bed is %s", action, b.Status)
}

func (r *BedRepository) CreateForTent(ctx context.Context, tenantID, tentID uuid.UUID, count int) ([]domain.Bed, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op after Commit

	// Row lock serialises concurrent provisioning of the same tent so the
	// capacity check below cannot be raced.
	var capacity int
	if err := tx.QueryRow(ctx,
		`SELECT capacity FROM tents WHERE tenant_id = $1 AND id = $2 FOR UPDATE`,
		tenantID, tentID).Scan(&capacity); err != nil {
		return nil, mapErr(err, "tent not found")
	}
	var existing int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM beds WHERE tenant_id = $1 AND tent_id = $2`,
		tenantID, tentID).Scan(&existing); err != nil {
		return nil, err
	}
	if existing+count > capacity {
		return nil, domain.Conflict("tent capacity is %d and it already has %d beds", capacity, existing)
	}

	beds := make([]domain.Bed, 0, count)
	batch := &pgx.Batch{}
	for i := 0; i < count; i++ {
		b, err := domain.NewBed(tenantID, tentID)
		if err != nil {
			return nil, fmt.Errorf("new bed: %w", err)
		}
		beds = append(beds, b)
		batch.Queue(
			`INSERT INTO beds (id, tenant_id, tent_id, status, qr_hash, created_at, updated_at)
			 VALUES ($1, $2, $3, 'empty', $4, $5, $6)`,
			b.ID, b.TenantID, b.TentID, b.QRHash, b.CreatedAt, b.UpdatedAt)
	}
	br := tx.SendBatch(ctx, batch)
	for range beds {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return nil, mapErr(err, "tent not found")
		}
	}
	if err := br.Close(); err != nil {
		return nil, mapErr(err, "tent not found")
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return beds, nil
}
