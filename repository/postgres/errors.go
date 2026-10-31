// Package postgres implements the domain repository ports with pgx.
package postgres

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/arafat-360/arafa-360-be/domain"
)

// Postgres SQLSTATE classes we translate into domain errors.
const (
	pgForeignKeyViolation = "23503"
	pgUniqueViolation     = "23505"
	pgCheckViolation      = "23514"
)

// mapErr turns driver errors into domain errors. Anything unrecognised is
// returned untouched so it surfaces as a 500 and is reported to Sentry.
func mapErr(err error, notFound string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.NotFound("%s", notFound)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgUniqueViolation:
			switch pgErr.ConstraintName {
			case "beds_tenant_pilgrim_uidx":
				return domain.Conflict("pilgrim already holds a bed")
			case "zones_tenant_id_name_key":
				return domain.Conflict("a zone with this name already exists")
			default:
				return domain.Conflict("resource already exists")
			}
		case pgForeignKeyViolation:
			return domain.Invalid("referenced resource does not exist in this tenant")
		case pgCheckViolation:
			return domain.Invalid("value violates a data constraint (%s)", pgErr.ConstraintName)
		}
	}
	return err
}
