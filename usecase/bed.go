package usecase

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/arafat-360/arafa-360-be/domain"
)

// AsyncExecutor runs fire-and-forget side effects on a bounded pool.
// TrySubmit must never block; it returns false when the work is shed.
type AsyncExecutor interface {
	TrySubmit(task func(ctx context.Context)) bool
}

// BedUsecase serves the QR-scan hot path.
//
// The scan result (the DB write) is on the request path and is the source of
// truth. Telling the dashboard about it is a side effect: it goes through the
// bounded worker pool and is shed under extreme load rather than slowing
// scans down or exhausting memory.
type BedUsecase struct {
	beds   domain.BedRepository
	pub    domain.EventPublisher
	async  AsyncExecutor
	log    *slog.Logger
	pubTTL time.Duration
}

func NewBedUsecase(beds domain.BedRepository, pub domain.EventPublisher, async AsyncExecutor, log *slog.Logger) *BedUsecase {
	return &BedUsecase{beds: beds, pub: pub, async: async, log: log, pubTTL: 3 * time.Second}
}

func (u *BedUsecase) Lookup(ctx context.Context, tenantID uuid.UUID, qrHash string) (domain.Bed, error) {
	if !domain.ValidQRHash(qrHash) {
		return domain.Bed{}, domain.Invalid("malformed qr_hash")
	}
	return u.beds.GetByQRHash(ctx, tenantID, qrHash)
}

func (u *BedUsecase) CheckIn(ctx context.Context, tenantID uuid.UUID, qrHash string, pilgrimID uuid.UUID) (domain.Bed, error) {
	if !domain.ValidQRHash(qrHash) {
		return domain.Bed{}, domain.Invalid("malformed qr_hash")
	}
	if pilgrimID == uuid.Nil {
		return domain.Bed{}, domain.Invalid("pilgrim_id is required")
	}
	b, err := u.beds.CheckIn(ctx, tenantID, qrHash, pilgrimID)
	if err != nil {
		return domain.Bed{}, err
	}
	u.emit(domain.Event{
		Type: domain.EventBedCheckedIn, Severity: domain.SeverityInfo,
		TenantID: tenantID, TentID: &b.TentID, BedID: &b.ID, PilgrimID: b.PilgrimID,
		OccurredAt: b.UpdatedAt,
	})
	return b, nil
}

func (u *BedUsecase) CheckOut(ctx context.Context, tenantID uuid.UUID, qrHash string) (domain.Bed, error) {
	if !domain.ValidQRHash(qrHash) {
		return domain.Bed{}, domain.Invalid("malformed qr_hash")
	}
	b, err := u.beds.CheckOut(ctx, tenantID, qrHash)
	if err != nil {
		return domain.Bed{}, err
	}
	u.emit(domain.Event{
		Type: domain.EventBedCheckedOut, Severity: domain.SeverityInfo,
		TenantID: tenantID, TentID: &b.TentID, BedID: &b.ID,
		OccurredAt: b.UpdatedAt,
	})
	return b, nil
}

func (u *BedUsecase) emit(e domain.Event) {
	e.ID = uuid.New()
	ok := u.async.TrySubmit(func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, u.pubTTL)
		defer cancel()
		if err := u.pub.Publish(ctx, e); err != nil {
			u.log.Error("publish bed event", "error", err, "tenant_id", e.TenantID, "type", e.Type)
		}
	})
	if !ok {
		u.log.Warn("worker pool saturated, bed event shed", "tenant_id", e.TenantID, "type", e.Type)
	}
}
