package domain

import (
	"time"

	"github.com/google/uuid"
)

type EventType string

const (
	EventIncidentReported EventType = "incident.reported"
	EventBedCheckedIn     EventType = "bed.checked_in"
	EventBedCheckedOut    EventType = "bed.checked_out"
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

func (s Severity) Valid() bool {
	switch s {
	case SeverityInfo, SeverityWarning, SeverityCritical:
		return true
	}
	return false
}

type IncidentCategory string

const (
	IncidentACFailure     IncidentCategory = "ac_failure"
	IncidentPowerOutage   IncidentCategory = "power_outage"
	IncidentWaterShortage IncidentCategory = "water_shortage"
	IncidentMedical       IncidentCategory = "medical"
	IncidentFire          IncidentCategory = "fire"
	IncidentOvercrowding  IncidentCategory = "overcrowding"
	IncidentOther         IncidentCategory = "other"
)

func (c IncidentCategory) Valid() bool {
	switch c {
	case IncidentACFailure, IncidentPowerOutage, IncidentWaterShortage,
		IncidentMedical, IncidentFire, IncidentOvercrowding, IncidentOther:
		return true
	}
	return false
}

// Event is the envelope broadcast to the Command Center dashboard. It is
// serialised to JSON exactly once (by the publisher) and fanned out as raw
// bytes by every instance's WebSocket hub.
type Event struct {
	ID         uuid.UUID        `json:"id"`
	TenantID   uuid.UUID        `json:"tenant_id"`
	Type       EventType        `json:"type"`
	Severity   Severity         `json:"severity"`
	Category   IncidentCategory `json:"category,omitempty"`
	ZoneID     *uuid.UUID       `json:"zone_id,omitempty"`
	TentID     *uuid.UUID       `json:"tent_id,omitempty"`
	BedID      *uuid.UUID       `json:"bed_id,omitempty"`
	PilgrimID  *uuid.UUID       `json:"pilgrim_id,omitempty"`
	Message    string           `json:"message,omitempty"`
	OccurredAt time.Time        `json:"occurred_at"`
}
