// Package usecase contains the application rules. It orchestrates domain
// entities through the ports declared in package domain and knows nothing
// about HTTP, SQL or Redis.
package usecase

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/arafat-360/arafa-360-be/domain"
)

// TenantUsecase resolves and authorises the tenant of every request. Every
// API call passes through here, so successful lookups are cached in-process.
// Suspending a tenant therefore takes effect within one TTL.
type TenantUsecase struct {
	repo domain.TenantRepository
	ttl  time.Duration

	mu    sync.RWMutex
	cache map[uuid.UUID]cached
}

type cached struct {
	tenant  domain.Tenant
	expires time.Time
}

func NewTenantUsecase(repo domain.TenantRepository, ttl time.Duration) *TenantUsecase {
	return &TenantUsecase{repo: repo, ttl: ttl, cache: make(map[uuid.UUID]cached)}
}

// Resolve returns an active tenant or a domain error (NotFound / Forbidden).
func (u *TenantUsecase) Resolve(ctx context.Context, id uuid.UUID) (domain.Tenant, error) {
	now := time.Now()
	u.mu.RLock()
	c, ok := u.cache[id]
	u.mu.RUnlock()
	if ok && now.Before(c.expires) {
		return c.tenant, nil
	}

	t, err := u.repo.GetByID(ctx, id)
	if err != nil {
		return domain.Tenant{}, err
	}
	if !t.Active() {
		return domain.Tenant{}, domain.Forbidden("tenant is suspended")
	}

	u.mu.Lock()
	u.cache[id] = cached{tenant: t, expires: now.Add(u.ttl)}
	u.mu.Unlock()
	return t, nil
}
