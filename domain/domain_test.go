package domain

import (
	"errors"
	"testing"

	"github.com/google/uuid"
)

func TestPolygonValidate(t *testing.T) {
	ok := Polygon{{21.35, 39.98}, {21.35, 39.99}, {21.34, 39.99}}
	cases := map[string]struct {
		p       Polygon
		wantErr bool
	}{
		"valid triangle":   {ok, false},
		"too few vertices": {ok[:2], true},
		"empty":            {nil, true},
		"lat out of range": {Polygon{{91, 0}, {0, 0}, {1, 1}}, true},
		"lng out of range": {Polygon{{0, 181}, {0, 0}, {1, 1}}, true},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := c.p.Validate()
			if (err != nil) != c.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, c.wantErr)
			}
			if err != nil && !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
		})
	}
}

func TestNewTentValidation(t *testing.T) {
	tenant, zone := uuid.New(), uuid.New()
	if _, err := NewTent(tenant, zone, TentStandard, 10); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		typ TentType
		cap int
	}{{"igloo", 10}, {TentVIP, 0}, {TentVIP, MaxTentCapacity + 1}} {
		if _, err := NewTent(tenant, zone, c.typ, c.cap); !errors.Is(err, ErrInvalid) {
			t.Fatalf("NewTent(%q,%d) err = %v, want ErrInvalid", c.typ, c.cap, err)
		}
	}
}

func TestNewBedStartsEmptyWithUnguessableUniqueQR(t *testing.T) {
	a, err := NewBed(uuid.New(), uuid.New())
	if err != nil {
		t.Fatal(err)
	}
	b, _ := NewBed(uuid.New(), uuid.New())
	if a.Status != BedEmpty || a.PilgrimID != nil {
		t.Fatalf("new bed must be empty and unassigned: %+v", a)
	}
	if !ValidQRHash(a.QRHash) || a.QRHash == b.QRHash {
		t.Fatalf("bad or duplicate QR hash: %q %q", a.QRHash, b.QRHash)
	}
}

func TestValidQRHashRejectsMalformed(t *testing.T) {
	for _, s := range []string{"", "abc", "G" + string(make([]byte, 63)), "' OR 1=1 --"} {
		if ValidQRHash(s) {
			t.Fatalf("%q must be rejected", s)
		}
	}
}

func TestErrorKindsAreMatchableAndCauseIsReachable(t *testing.T) {
	cause := errors.New("redis down")
	err := OverloadedBy(cause, "retry")
	if !errors.Is(err, ErrOverloaded) || !errors.Is(err, cause) {
		t.Fatal("kind and cause must both be reachable with errors.Is")
	}
	if err.Error() != "retry" {
		t.Fatalf("client message must not leak the cause: %q", err.Error())
	}
}
