package usecase

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/arafat-360/arafa-360-be/domain"
)

type ReportIncidentInput struct {
	TentID   uuid.UUID
	Category domain.IncidentCategory
	Severity domain.Severity
	Message  string
}

// IncidentUsecase turns a pilgrim's report into a Command Center alert.
//
// Unlike bed events, an incident IS the product: it is published
// synchronously and the caller learns whether it was delivered to the bus.
// A failure is returned (HTTP 503) so the client can retry; it is never
// silently dropped.
type IncidentUsecase struct {
	tents      domain.TentRepository
	pub        domain.EventPublisher
	publishTTL time.Duration
}

func NewIncidentUsecase(tents domain.TentRepository, pub domain.EventPublisher) *IncidentUsecase {
	return &IncidentUsecase{tents: tents, pub: pub, publishTTL: 2 * time.Second}
}

func (u *IncidentUsecase) Report(ctx context.Context, tenantID uuid.UUID, in ReportIncidentInput) (domain.Event, error) {
	if !in.Category.Valid() {
		return domain.Event{}, domain.Invalid("unknown category")
	}
	if !in.Severity.Valid() {
		return domain.Event{}, domain.Invalid("severity must be info, warning or critical")
	}
	msg := strings.TrimSpace(in.Message)
	if len(msg) > 500 {
		return domain.Event{}, domain.Invalid("message too long (max 500 characters)")
	}

	// Tenant-scoped lookup: a tent from another agency is "not found", and
	// it gives us the zone so the dashboard can place the alert on the map.
	tent, err := u.tents.GetByID(ctx, tenantID, in.TentID)
	if err != nil {
		return domain.Event{}, err
	}

	ev := domain.Event{
		ID:         uuid.New(),
		TenantID:   tenantID,
		Type:       domain.EventIncidentReported,
		Severity:   in.Severity,
		Category:   in.Category,
		ZoneID:     &tent.ZoneID,
		TentID:     &tent.ID,
		Message:    msg,
		OccurredAt: time.Now().UTC(),
	}

	pubCtx, cancel := context.WithTimeout(ctx, u.publishTTL)
	defer cancel()
	if err := u.pub.Publish(pubCtx, ev); err != nil {
		return domain.Event{}, domain.OverloadedBy(err, "could not reach the command center, retry")
	}
	return ev, nil
}
