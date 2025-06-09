package entlock

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/bbedward/gocron-ent-lock/v2/ent"
	"github.com/bbedward/gocron-ent-lock/v2/ent/cronjoblock"
	"github.com/go-co-op/gocron/v2"
)

var _ gocron.Locker = (*EntLocker)(nil)

type EntLocker struct {
	client        *ent.Client
	worker        string
	ttl           time.Duration
	interval      time.Duration
	jobIdentifier func(ctx context.Context, key string) string

	closed atomic.Bool
}

// NewEntLocker Creates a new EntLocker
func NewEntLocker(client *ent.Client, worker string, options ...LockOption) (*EntLocker, error) {
	if client == nil {
		return nil, ErrEntCantBeNull
	}
	if worker == "" {
		return nil, ErrWorkerIsRequired
	}

	el := &EntLocker{
		client:   client,
		worker:   worker,
		ttl:      defaultTTL,
		interval: defaultCleanInterval,
	}
	el.jobIdentifier = defaultJobIdentifier(defaultPrecision)
	for _, option := range options {
		option(el)
	}

	go func() {
		ticker := time.NewTicker(el.interval)
		defer ticker.Stop()

		for range ticker.C {
			if el.closed.Load() {
				return
			}

			el.cleanExpiredRecords()
		}
	}()

	return el, nil
}

func (e *EntLocker) Lock(ctx context.Context, key string) (gocron.Lock, error) {
	ji := e.jobIdentifier(ctx, key)

	cjl, err := e.client.CronJobLock.
		Create().
		SetJobName(key).
		SetJobIdentifier(ji).
		SetWorker(e.worker).
		SetStatus(StatusRunning).
		Save(ctx)
	if err != nil {
		return nil, err
	}
	return &entLock{client: e.client, id: cjl.ID}, nil
}

func (e *EntLocker) Close() {
	e.closed.Store(true)
}

func (e *EntLocker) cleanExpiredRecords() {
	ctx := context.Background()
	_, err := e.client.CronJobLock.
		Delete().
		Where(
			cronjoblock.And(
				cronjoblock.UpdatedAtLT(time.Now().Add(-e.ttl)),
				cronjoblock.StatusEQ(StatusFinished),
			),
		).
		Exec(ctx)
	if err != nil {
		// Log error but don't fail - this is a background cleanup operation
		// In production, you might want to use a proper logger here
		_ = err
	}
}

var _ gocron.Lock = (*entLock)(nil)

type entLock struct {
	client *ent.Client
	// id the id that lock a particular job
	id int
}

func (e *entLock) Unlock(ctx context.Context) error {
	return e.client.CronJobLock.
		UpdateOneID(e.id).
		SetStatus(StatusFinished).
		Exec(ctx)
}
