package entlock

import (
	"context"
	"testing"
	"time"

	"github.com/bbedward/gocron-ent-lock/v2/ent"
	"github.com/bbedward/gocron-ent-lock/v2/ent/cronjoblock"
	"github.com/bbedward/gocron-ent-lock/v2/ent/enttest"
	"github.com/go-co-op/gocron/v2"
	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEntLocker_Validation(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		client *ent.Client
		worker string
		err    string
	}{
		"client is nil":   {client: nil, worker: "local", err: ErrEntCantBeNull.Error()},
		"worker is empty": {client: &ent.Client{}, worker: "", err: ErrWorkerIsRequired.Error()},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := NewEntLocker(tc.client, tc.worker)
			if assert.Error(t, err) {
				assert.ErrorContains(t, err, tc.err)
			}
		})
	}
}

func setupTestClient(ctx context.Context, t *testing.T) (*ent.Client, func()) {
	client := enttest.Open(t, "sqlite3", "file:ent?mode=memory&cache=shared&_fk=1")

	cleanup := func() {
		client.Close()
	}

	return client, cleanup
}

func TestEnableDistributedLocking(t *testing.T) {
	ctx := context.Background()
	client, cleanup := setupTestClient(ctx, t)
	defer cleanup()

	resultChan := make(chan int, 10)
	f := func(schedulerInstance int) {
		resultChan <- schedulerInstance
		println(time.Now().Truncate(defaultPrecision).Format("2006-01-02 15:04:05.000"))
	}

	l1, err := NewEntLocker(client, "s1")
	require.NoError(t, err)
	s1, schErr := gocron.NewScheduler(gocron.WithLocation(time.UTC), gocron.WithDistributedLocker(l1))
	require.NoError(t, schErr)

	_, err = s1.NewJob(gocron.DurationJob(1*time.Second), gocron.NewTask(f, 1))
	require.NoError(t, err)

	l2, err := NewEntLocker(client, "s2")
	require.NoError(t, err)
	s2, schErr := gocron.NewScheduler(gocron.WithLocation(time.UTC), gocron.WithDistributedLocker(l2))
	require.NoError(t, schErr)
	_, err = s2.NewJob(gocron.DurationJob(1*time.Second), gocron.NewTask(f, 2))
	require.NoError(t, err)

	s1.Start()
	s2.Start()

	time.Sleep(4500 * time.Millisecond)

	require.NoError(t, s1.Shutdown())
	require.NoError(t, s2.Shutdown())
	close(resultChan)

	var results []int
	for r := range resultChan {
		results = append(results, r)
	}
	assert.Len(t, results, 4)

	allCronJobs, err := client.CronJobLock.Query().All(ctx)
	require.NoError(t, err)
	assert.Equal(t, len(results), len(allCronJobs))
}

func TestEnableDistributedLocking_DifferentJob(t *testing.T) {
	ctx := context.Background()
	client, cleanup := setupTestClient(ctx, t)
	defer cleanup()

	resultChan := make(chan int, 10)
	f := func(schedulerInstance int) {
		resultChan <- schedulerInstance
	}

	result2Chan := make(chan int, 10)
	f2 := func(schedulerInstance int) {
		result2Chan <- schedulerInstance
	}

	l1, err := NewEntLocker(client, "s1")
	require.NoError(t, err)
	s1, schErr := gocron.NewScheduler(gocron.WithLocation(time.UTC), gocron.WithDistributedLocker(l1))
	require.NoError(t, schErr)
	_, err = s1.NewJob(gocron.DurationJob(1*time.Second), gocron.NewTask(f, 1), gocron.WithName("f"))
	require.NoError(t, err)
	_, err = s1.NewJob(gocron.DurationJob(1*time.Second), gocron.NewTask(f2, 1), gocron.WithName("f2"))
	require.NoError(t, err)

	l2, err := NewEntLocker(client, "s2")
	require.NoError(t, err)
	s2, schErr := gocron.NewScheduler(gocron.WithLocation(time.UTC), gocron.WithDistributedLocker(l2))
	require.NoError(t, schErr)
	_, err = s2.NewJob(gocron.DurationJob(1*time.Second), gocron.NewTask(f, 2), gocron.WithName("f"))
	require.NoError(t, err)
	_, err = s2.NewJob(gocron.DurationJob(1*time.Second), gocron.NewTask(f2, 2), gocron.WithName("f2"))
	require.NoError(t, err)

	l3, err := NewEntLocker(client, "s3")
	require.NoError(t, err)
	s3, schErr := gocron.NewScheduler(gocron.WithLocation(time.UTC), gocron.WithDistributedLocker(l3))
	require.NoError(t, schErr)

	_, err = s3.NewJob(gocron.DurationJob(1*time.Second), gocron.NewTask(f, 3), gocron.WithName("f"))
	require.NoError(t, err)
	_, err = s3.NewJob(gocron.DurationJob(1*time.Second), gocron.NewTask(f2, 3), gocron.WithName("f2"))
	require.NoError(t, err)

	s1.Start()
	s2.Start()
	s3.Start()

	time.Sleep(4500 * time.Millisecond)

	require.NoError(t, s1.Shutdown())
	require.NoError(t, s2.Shutdown())
	require.NoError(t, s3.Shutdown())
	close(resultChan)
	close(result2Chan)

	var results []int
	for r := range resultChan {
		results = append(results, r)
	}
	assert.Len(t, results, 4, "f is expected 4 times")
	var results2 []int
	for r := range result2Chan {
		results2 = append(results2, r)
	}
	assert.Len(t, results2, 4, "f2 is expected 4 times")

	allCronJobs, err := client.CronJobLock.Query().All(ctx)
	require.NoError(t, err)
	assert.Equal(t, len(results)+len(results2), len(allCronJobs))
}

func TestJobReturningExceptionWhenUnique(t *testing.T) {
	precision := 60 * time.Minute
	tests := map[string]struct {
		ji         string
		lockOption []LockOption
	}{
		"default job identifier": {
			ji:         defaultJobIdentifier(precision)(context.Background(), "key"),
			lockOption: []LockOption{WithDefaultJobIdentifier(precision)},
		},
		"override job identifier with hardcoded name": {
			ji: "hardcoded",
			lockOption: []LockOption{WithJobIdentifier(func(_ context.Context, _ string) string {
				return "hardcoded"
			})},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			client, cleanup := setupTestClient(ctx, t)
			defer cleanup()

			// creating an entry to force the unique identifier error
			_, err := client.CronJobLock.
				Create().
				SetJobName("job").
				SetJobIdentifier(tc.ji).
				SetWorker("local").
				SetStatus(StatusRunning).
				Save(ctx)
			require.NoError(t, err)

			l, _ := NewEntLocker(client, "local", tc.lockOption...)
			_, lerr := l.Lock(ctx, "job")
			assert.True(t, ent.IsConstraintError(lerr))
		})
	}
}

func TestHandleTTL(t *testing.T) {
	ctx := context.Background()
	client, cleanup := setupTestClient(ctx, t)
	defer cleanup()

	l1, err := NewEntLocker(client, "s1", WithTTL(1*time.Second))
	require.NoError(t, err)

	s1, schErr := gocron.NewScheduler(gocron.WithLocation(time.UTC), gocron.WithDistributedLocker(l1))
	require.NoError(t, schErr)

	_, err = s1.NewJob(gocron.DurationJob(1*time.Second), gocron.NewTask(func() {}))
	require.NoError(t, err)

	s1.Start()

	time.Sleep(3500 * time.Millisecond)

	require.NoError(t, s1.Shutdown())

	allCronJobs, err := client.CronJobLock.Query().All(ctx)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(allCronJobs), 3)

	// wait for data to expire
	time.Sleep(1500 * time.Millisecond)
	l1.cleanExpiredRecords()

	allCronJobs, err = client.CronJobLock.Query().All(ctx)
	require.NoError(t, err)
	assert.Equal(t, 0, len(allCronJobs))
}

func TestEntLock_Unlock(t *testing.T) {
	ctx := context.Background()
	client, cleanup := setupTestClient(ctx, t)
	defer cleanup()

	// Create a lock record
	cjl, err := client.CronJobLock.
		Create().
		SetJobName("test-job").
		SetJobIdentifier("test-identifier").
		SetWorker("test-worker").
		SetStatus(StatusRunning).
		Save(ctx)
	require.NoError(t, err)

	// Create entLock instance and unlock
	lock := &entLock{client: client, id: cjl.ID}
	err = lock.Unlock(ctx)
	require.NoError(t, err)

	// Verify the status was updated
	updatedLock, err := client.CronJobLock.Get(ctx, cjl.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusFinished, updatedLock.Status)
}

func TestCleanExpiredRecords(t *testing.T) {
	ctx := context.Background()
	client, cleanup := setupTestClient(ctx, t)
	defer cleanup()

	locker, err := NewEntLocker(client, "test-worker", WithTTL(1*time.Second))
	require.NoError(t, err)

	// Create some old finished records
	pastTime := time.Now().Add(-2 * time.Second)
	_, err = client.CronJobLock.
		Create().
		SetJobName("old-job").
		SetJobIdentifier("old-identifier").
		SetWorker("test-worker").
		SetStatus(StatusFinished).
		SetCreatedAt(pastTime).
		SetUpdatedAt(pastTime).
		Save(ctx)
	require.NoError(t, err)

	// Create a recent finished record (should not be deleted)
	_, err = client.CronJobLock.
		Create().
		SetJobName("recent-job").
		SetJobIdentifier("recent-identifier").
		SetWorker("test-worker").
		SetStatus(StatusFinished).
		Save(ctx)
	require.NoError(t, err)

	// Create a running record (should not be deleted)
	_, err = client.CronJobLock.
		Create().
		SetJobName("running-job").
		SetJobIdentifier("running-identifier").
		SetWorker("test-worker").
		SetStatus(StatusRunning).
		Save(ctx)
	require.NoError(t, err)

	// Run cleanup
	locker.cleanExpiredRecords()

	// Check results
	finishedJobs, err := client.CronJobLock.Query().
		Where(cronjoblock.StatusEQ(StatusFinished)).
		All(ctx)
	require.NoError(t, err)
	assert.Len(t, finishedJobs, 1, "Should have 1 recent finished job remaining")

	runningJobs, err := client.CronJobLock.Query().
		Where(cronjoblock.StatusEQ(StatusRunning)).
		All(ctx)
	require.NoError(t, err)
	assert.Len(t, runningJobs, 1, "Should have 1 running job remaining")
}
