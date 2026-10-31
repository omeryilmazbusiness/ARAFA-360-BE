package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// LatLng is a WGS84 coordinate.
type LatLng struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

// Polygon is an ordered ring of vertices. It is stored as JSONB so the stack
// runs on any managed Postgres (incl. Railway) without PostGIS. If spatial
// queries (point-in-polygon, nearest tent) become hot, migrate the column to
// geography(Polygon,4326); the domain type does not change.
type Polygon []LatLng

const maxPolygonVertices = 1000

func (p Polygon) Validate() error {
	if len(p) < 3 {
		return Invalid("coordinates_polygon needs at least 3 vertices")
	}
	if len(p) > maxPolygonVertices {
		return Invalid("coordinates_polygon has too many vertices (max %d)", maxPolygonVertices)
	}
	for i, v := range p {
		if v.Lat < -90 || v.Lat > 90 || v.Lng < -180 || v.Lng > 180 {
			return Invalid("coordinates_polygon vertex %d is out of range", i)
		}
	}
	return nil
}

// Zone is an area of Arafat (e.g. a camp sector) owned by one tenant.
type Zone struct {
	ID                 uuid.UUID `db:"id"                  json:"id"`
	TenantID           uuid.UUID `db:"tenant_id"           json:"tenant_id"`
	Name               string    `db:"name"                json:"name"`
	CoordinatesPolygon Polygon   `db:"coordinates_polygon" json:"coordinates_polygon"`
	CreatedAt          time.Time `db:"created_at"          json:"created_at"`
	UpdatedAt          time.Time `db:"updated_at"          json:"updated_at"`
}

func NewZone(tenantID uuid.UUID, name string, polygon Polygon) (Zone, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return Zone{}, Invalid("name is required (max 120 characters)")
	}
	if err := polygon.Validate(); err != nil {
		return Zone{}, err
	}
	now := time.Now().UTC()
	return Zone{
		ID:                 uuid.New(),
		TenantID:           tenantID,
		Name:               name,
		CoordinatesPolygon: polygon,
		CreatedAt:          now,
		UpdatedAt:          now,
	}, nil
}
