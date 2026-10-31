-- ARAFA-360 core schema. Shared-schema multi-tenancy: every operational table
-- carries tenant_id and every index that serves a hot path leads with it.

CREATE TABLE tenants (
    id          uuid        PRIMARY KEY,
    name        text        NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    slug        text        NOT NULL UNIQUE,
    status      text        NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended')),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE zones (
    id                  uuid        PRIMARY KEY,
    tenant_id           uuid        NOT NULL REFERENCES tenants (id) ON DELETE RESTRICT,
    name                text        NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
    coordinates_polygon jsonb       NOT NULL CHECK (jsonb_typeof(coordinates_polygon) = 'array'),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    -- Target of the composite foreign keys below; makes cross-tenant links impossible.
    UNIQUE (tenant_id, id),
    UNIQUE (tenant_id, name)
);

-- Tent type is CHECK-constrained text rather than a PG enum: new tent types
-- can be added without ALTER TYPE (which cannot run inside a transaction on
-- older servers). bed_status is a real enum because it is a closed state machine.
CREATE TABLE tents (
    id          uuid        PRIMARY KEY,
    tenant_id   uuid        NOT NULL,
    zone_id     uuid        NOT NULL,
    type        text        NOT NULL CHECK (type IN ('standard', 'vip', 'medical', 'service')),
    capacity    integer     NOT NULL CHECK (capacity BETWEEN 1 AND 5000),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, id),
    FOREIGN KEY (tenant_id, zone_id) REFERENCES zones (tenant_id, id) ON DELETE RESTRICT
);

CREATE INDEX tents_tenant_zone_idx ON tents (tenant_id, zone_id);

CREATE TYPE bed_status AS ENUM ('empty', 'occupied', 'maintenance');

CREATE TABLE beds (
    id          uuid        PRIMARY KEY,
    tenant_id   uuid        NOT NULL,
    tent_id     uuid        NOT NULL,
    -- pilgrim_id references the pilgrim registry (a future service/table); kept as a bare uuid on purpose.
    pilgrim_id  uuid,
    status      bed_status  NOT NULL DEFAULT 'empty',
    qr_hash     text        NOT NULL CHECK (qr_hash ~ '^[0-9a-f]{64}$'),
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (tenant_id, tent_id) REFERENCES tents (tenant_id, id) ON DELETE RESTRICT,
    -- A bed has a pilgrim if and only if it is occupied.
    CONSTRAINT beds_pilgrim_iff_occupied CHECK ((status = 'occupied') = (pilgrim_id IS NOT NULL))
);

-- QR scan hot path: single index probe.
CREATE UNIQUE INDEX beds_tenant_qr_hash_uidx ON beds (tenant_id, qr_hash);
-- One pilgrim can hold at most one bed per tenant (also makes check-in race-safe).
CREATE UNIQUE INDEX beds_tenant_pilgrim_uidx ON beds (tenant_id, pilgrim_id) WHERE pilgrim_id IS NOT NULL;
-- Occupancy dashboards: "how many empty beds in tent X".
CREATE INDEX beds_tenant_tent_status_idx ON beds (tenant_id, tent_id, status);
