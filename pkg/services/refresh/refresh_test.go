package refresh_test

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CerealKiller97/preuzmi.me/pkg/services/provider"
	"github.com/CerealKiller97/preuzmi.me/pkg/services/refresh"
	"github.com/stretchr/testify/require"
)

// fakeProvider stands in for a real provider so the tests never touch the
// network or anybody's account.
type fakeProvider struct {
	err   error
	calls atomic.Int32
	delay time.Duration
}

var _ provider.Interface = (*fakeProvider)(nil)

func (f *fakeProvider) DownloadReceipt() error {
	f.calls.Add(1)

	if f.delay > 0 {
		time.Sleep(f.delay)
	}

	return f.err
}

func TestRunReportsEveryProviderIndependently(t *testing.T) {
	// Arrange
	assert := require.New(t)
	boom := errors.New("login failed")
	providers := map[string]provider.Interface{
		"mts":      &fakeProvider{},
		"esanduce": &fakeProvider{err: boom},
	}

	// Act
	results := refresh.Run(providers)

	// Assert: one failure must not suppress the other provider's success.
	assert.Len(results, 2)
	assert.Equal("esanduce", results[0].Provider)
	assert.False(results[0].OK)
	assert.Equal("login failed", results[0].Error)
	assert.Equal("mts", results[1].Provider)
	assert.True(results[1].OK)
	assert.Empty(results[1].Error)
}

func TestStartRefusesConcurrentRuns(t *testing.T) {
	// Arrange
	assert := require.New(t)
	svc, err := refresh.New(t.TempDir())
	assert.NoError(err)

	slow := &fakeProvider{delay: 300 * time.Millisecond}
	providers := map[string]provider.Interface{"mts": slow}

	// Act
	_, err = svc.Start(providers, nil)
	assert.NoError(err)

	_, secondErr := svc.Start(providers, nil)

	// Assert: the second call is rejected rather than logging in twice.
	assert.ErrorIs(secondErr, refresh.ErrAlreadyRunning)
	assert.True(svc.State().Running)

	// The single run still completes and is recorded.
	assert.Eventually(func() bool {
		return !svc.State().Running
	}, 3*time.Second, 20*time.Millisecond)

	assert.Equal(int32(1), slow.calls.Load())
	assert.NotZero(svc.State().FinishedAt)
}

func TestStateSurvivesRestartAndIsNeverLeftRunning(t *testing.T) {
	// Arrange
	assert := require.New(t)
	dir := t.TempDir()
	svc, err := refresh.New(dir)
	assert.NoError(err)

	_, err = svc.Start(map[string]provider.Interface{"mts": &fakeProvider{}}, nil)
	assert.NoError(err)
	assert.Eventually(func() bool {
		return !svc.State().Running
	}, 3*time.Second, 20*time.Millisecond)

	finishedAt := svc.State().FinishedAt

	// Act: a fresh service reading the same directory, as after a restart.
	reloaded, err := refresh.New(dir)
	assert.NoError(err)

	// Assert
	assert.Equal(finishedAt, reloaded.State().FinishedAt)
	assert.False(reloaded.State().Running)
	assert.Len(reloaded.State().Results, 1)
	assert.True(reloaded.State().Results[0].OK)
}

func TestRunOncePersistsSoTheDashboardSeesTheRun(t *testing.T) {
	// Arrange
	assert := require.New(t)
	dir := t.TempDir()
	svc, err := refresh.New(dir)
	assert.NoError(err)

	// Act: the `checks` command runs synchronously and records the run.
	results, err := svc.RunOnce(map[string]provider.Interface{"mts": &fakeProvider{}})
	assert.NoError(err)
	assert.Len(results, 1)
	assert.False(svc.State().Running)
	assert.NotZero(svc.State().FinishedAt)

	// Assert: a fresh service (the running server) reads the advanced time.
	reloaded, err := refresh.New(dir)
	assert.NoError(err)
	assert.Equal(svc.State().FinishedAt, reloaded.State().FinishedAt)
	assert.Len(reloaded.State().Results, 1)
	assert.True(reloaded.State().Results[0].OK)
}

func TestSyncFromDiskAdoptsANewerOutOfProcessRun(t *testing.T) {
	// Arrange: a server-side service and a separate `checks`-side service
	// writing the same directory, standing in for the two processes.
	assert := require.New(t)
	dir := t.TempDir()

	server, err := refresh.New(dir)
	assert.NoError(err)
	assert.Zero(server.State().FinishedAt)

	checks, err := refresh.New(dir)
	assert.NoError(err)
	_, err = checks.RunOnce(map[string]provider.Interface{"mts": &fakeProvider{}})
	assert.NoError(err)

	// Act: the long-running server pulls the newer run off disk.
	server.SyncFromDisk()

	// Assert
	assert.Equal(checks.State().FinishedAt, server.State().FinishedAt)
	assert.Len(server.State().Results, 1)
	assert.False(server.State().Running)
}

func TestSyncFromDiskDoesNotRegressAnEqualOrNewerRun(t *testing.T) {
	// Arrange: this process just recorded a run, so disk and memory agree.
	assert := require.New(t)
	dir := t.TempDir()

	svc, err := refresh.New(dir)
	assert.NoError(err)
	_, err = svc.RunOnce(map[string]provider.Interface{"mts": &fakeProvider{}})
	assert.NoError(err)
	fresh := svc.State().FinishedAt

	// Act: syncing must not adopt a run that is not strictly newer, so the
	// recorded time and results survive untouched.
	svc.SyncFromDisk()

	// Assert
	assert.Equal(fresh, svc.State().FinishedAt)
	assert.Len(svc.State().Results, 1)
}

func TestNewOnEmptyDirectoryReportsNeverRun(t *testing.T) {
	// Arrange
	assert := require.New(t)

	// Act
	svc, err := refresh.New(t.TempDir())

	// Assert
	assert.NoError(err)
	assert.False(svc.State().Running)
	assert.Zero(svc.State().FinishedAt)
}
