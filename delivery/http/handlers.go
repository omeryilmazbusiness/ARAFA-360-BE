package http

import (
	"github.com/gofiber/fiber/v2"

	"github.com/arafat-360/arafa-360-be/domain"
	"github.com/arafat-360/arafa-360-be/usecase"
)

// Handlers are deliberately thin: parse -> call use case -> serialise.
// Nothing here is allowed to retain the *fiber.Ctx (or strings read from it)
// after returning; only parsed values cross into the use cases.

type Handlers struct {
	Inventory *usecase.InventoryUsecase
	Beds      *usecase.BedUsecase
	Incidents *usecase.IncidentUsecase
}

// ---- inventory -------------------------------------------------------------

func (h *Handlers) CreateZone(c *fiber.Ctx) error {
	var in struct {
		Name               string         `json:"name"`
		CoordinatesPolygon domain.Polygon `json:"coordinates_polygon"`
	}
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	z, err := h.Inventory.CreateZone(c.UserContext(), tenantID(c), in.Name, in.CoordinatesPolygon)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(z)
}

func (h *Handlers) CreateTent(c *fiber.Ctx) error {
	var in struct {
		ZoneID   string          `json:"zone_id"`
		Type     domain.TentType `json:"type"`
		Capacity int             `json:"capacity"`
	}
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	zoneID, err := parseUUID("zone_id", in.ZoneID)
	if err != nil {
		return err
	}
	t, err := h.Inventory.CreateTent(c.UserContext(), tenantID(c), zoneID, in.Type, in.Capacity)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(t)
}

func (h *Handlers) ProvisionBeds(c *fiber.Ctx) error {
	tentID, err := parseUUID("tent id", c.Params("id"))
	if err != nil {
		return err
	}
	var in struct {
		Count int `json:"count"`
	}
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	beds, err := h.Inventory.ProvisionBeds(c.UserContext(), tenantID(c), tentID, in.Count)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"beds": beds})
}

// ---- QR scans (hot path) ---------------------------------------------------
// The QR hash travels in the JSON body, not the URL, so it does not end up in
// access logs or Sentry transaction names.

type scanRequest struct {
	QRHash    string `json:"qr_hash"`
	PilgrimID string `json:"pilgrim_id"`
}

func (h *Handlers) ScanBed(c *fiber.Ctx) error {
	var in scanRequest
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	b, err := h.Beds.Lookup(c.UserContext(), tenantID(c), in.QRHash)
	if err != nil {
		return err
	}
	return c.JSON(b)
}

func (h *Handlers) CheckIn(c *fiber.Ctx) error {
	var in scanRequest
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	pilgrimID, err := parseUUID("pilgrim_id", in.PilgrimID)
	if err != nil {
		return err
	}
	b, err := h.Beds.CheckIn(c.UserContext(), tenantID(c), in.QRHash, pilgrimID)
	if err != nil {
		return err
	}
	return c.JSON(b)
}

func (h *Handlers) CheckOut(c *fiber.Ctx) error {
	var in scanRequest
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	b, err := h.Beds.CheckOut(c.UserContext(), tenantID(c), in.QRHash)
	if err != nil {
		return err
	}
	return c.JSON(b)
}

// ---- incidents -------------------------------------------------------------

func (h *Handlers) ReportIncident(c *fiber.Ctx) error {
	var in struct {
		TentID   string                  `json:"tent_id"`
		Category domain.IncidentCategory `json:"category"`
		Severity domain.Severity         `json:"severity"`
		Message  string                  `json:"message"`
	}
	if err := bindJSON(c, &in); err != nil {
		return err
	}
	tentID, err := parseUUID("tent_id", in.TentID)
	if err != nil {
		return err
	}
	ev, err := h.Incidents.Report(c.UserContext(), tenantID(c), usecase.ReportIncidentInput{
		TentID: tentID, Category: in.Category, Severity: in.Severity, Message: in.Message,
	})
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusAccepted).JSON(fiber.Map{"id": ev.ID, "status": "broadcast"})
}
