package domain

import (
	"context"

	"github.com/google/uuid"
)

// Ports. Defined here (the innermost layer) and implemented in repository/.
// Every method that touches tenant data takes tenantID explicitly: there is no
// way to read or write a row without naming its tenant.

type TenantRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (Tenant, error)
}

type ZoneRepository interface {
	Create(ctx context.Context, z Zone) error
}

type TentRepository interface {
	Create(ctx context.Context, t Tent) error
	GetByID(ctx context.Context, tenantID, id uuid.UUID) (Tent, error)
}

type BedRepository interface {
	// CreateForTent provisions `count` empty beds in a tent, failing with
	// ErrConflict if that would exceed the tent's capacity.
	CreateForTent(ctx context.Context, tenantID, tentID uuid.UUID, count int) ([]Bed, error)
	GetByQRHash(ctx context.Context, tenantID uuid.UUID, qrHash string) (Bed, error)
	// CheckIn atomically moves an empty bed to occupied. ErrConflict if the
	// bed is not empty or the pilgrim already holds another bed.
	CheckIn(ctx context.Context, tenantID uuid.UUID, qrHash string, pilgrimID uuid.UUID) (Bed, error)
	// CheckOut atomically frees an occupied bed.
	CheckOut(ctx context.Context, tenantID uuid.UUID, qrHash string) (Bed, error)
}

// EventPublisher sends an event to the Command Center.
type EventPublisher interface {
	Publish(ctx context.Context, e Event) error
}

// EventSubscriber delivers raw event payloads for ALL tenants to handler.
// It blocks until ctx is done or the subscription fails irrecoverably.
// Routing by tenant is the subscriber's job, so the hub never has to
// JSON-decode traffic.
type EventSubscriber interface {
	Subscribe(ctx context.Context, handler func(tenantID uuid.UUID, payload []byte)) error
}
