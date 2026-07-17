package sqlite

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/cschleiden/go-workflows/backend"
	"github.com/cschleiden/go-workflows/backend/test"
)

func Test_SqliteBackend(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	test.BackendTest(t, func(options ...backend.BackendOption) test.TestBackend {
		// Disable sticky workflow behavior for the test execution
		return NewInMemoryBackend(WithBackendOptions(append(options, backend.WithStickyTimeout(0))...), WithAutoVacuum())
		// return NewSqliteBackend("test.sqlite", WithBackendOptions(append(options, backend.WithStickyTimeout(0))...), WithAutoVacuum())
	}, func(b test.TestBackend) {
		// Ensure we close the database so the next test will get a clean in-memory db
		require.NoError(t, b.(*sqliteBackend).Close())
	})
}

func Test_EndToEndSqliteBackend(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}

	test.EndToEndBackendTest(t, func(options ...backend.BackendOption) test.TestBackend {
		// Disable sticky workflow behavior for the test execution
		return NewInMemoryBackend(WithBackendOptions(append(options, backend.WithStickyTimeout(0))...), WithAutoVacuum())
	}, func(b test.TestBackend) {
		// Ensure we close the database so the next test will get a clean in-memory db
		require.NoError(t, b.Close())
	})
}

func Test_SqliteBackend_WorkerName(t *testing.T) {
	t.Run("DefaultWorkerName", func(t *testing.T) {
		backend := NewInMemoryBackend()
		defer backend.Close()

		// The default worker name should be in the format "worker-<uuid>"
		require.Contains(t, backend.workerName, "worker-")
		require.Len(t, backend.workerName, 43) // "worker-" (7) + UUID (36)
	})

	t.Run("CustomWorkerName", func(t *testing.T) {
		customWorkerName := "test-worker-123"
		backend := NewInMemoryBackend(WithBackendOptions(backend.WithWorkerName(customWorkerName)))
		defer backend.Close()

		require.Equal(t, customWorkerName, backend.workerName)
	})

	t.Run("EmptyWorkerNameUsesDefault", func(t *testing.T) {
		backend := NewInMemoryBackend(WithBackendOptions(backend.WithWorkerName("")))
		defer backend.Close()

		// Empty worker name should fall back to UUID generation
		require.Contains(t, backend.workerName, "worker-")
		require.Len(t, backend.workerName, 43) // "worker-" (7) + UUID (36)
	})

	t.Run("CustomWorkerNameIsUsedInDatabase", func(t *testing.T) {
		customWorkerName := "integration-test-worker"
		backend := NewInMemoryBackend(WithBackendOptions(backend.WithWorkerName(customWorkerName)))
		defer backend.Close()

		// Verify the worker name is stored correctly
		require.Equal(t, customWorkerName, backend.workerName)
	})
}

// Test_SqliteBackend_ConnectionPragmas verifies that the per-connection PRAGMAs
// are applied through the DSN. This matters because the file-backed pool now
// recycles its single connection: busy_timeout is a per-connection setting, so
// it must live in the DSN to survive a connection being replaced. If it were
// applied via a one-off db.Exec it would silently drop back to 0 on the fresh
// connection, making SQLITE_BUSY failures far more likely.
func Test_SqliteBackend_ConnectionPragmas(t *testing.T) {
	dir := t.TempDir()
	b := NewSqliteBackend(filepath.Join(dir, "pragmas.sqlite"))
	defer b.Close()

	var busyTimeout int
	require.NoError(t, b.db.QueryRow("PRAGMA busy_timeout;").Scan(&busyTimeout))
	require.Equal(t, 5000, busyTimeout)

	var journalMode string
	require.NoError(t, b.db.QueryRow("PRAGMA journal_mode;").Scan(&journalMode))
	require.Equal(t, "wal", journalMode)
}

// Test_SqliteBackend_ConnectionRecycling verifies that the file-backed backend
// configures connection recycling so that a wedged connection can be reclaimed
// without a process restart, and that callers can override the defaults.
func Test_SqliteBackend_ConnectionRecycling(t *testing.T) {
	dir := t.TempDir()

	t.Run("Defaults", func(t *testing.T) {
		b := NewSqliteBackend(filepath.Join(dir, "defaults.sqlite"))
		defer b.Close()

		require.Equal(t, defaultConnMaxLifetime, b.options.ConnMaxLifetime)
		require.Equal(t, defaultConnMaxIdleTime, b.options.ConnMaxIdleTime)
	})

	t.Run("Overrides", func(t *testing.T) {
		b := NewSqliteBackend(
			filepath.Join(dir, "overrides.sqlite"),
			WithConnMaxLifetime(30*time.Second),
			WithConnMaxIdleTime(15*time.Second),
		)
		defer b.Close()

		require.Equal(t, 30*time.Second, b.options.ConnMaxLifetime)
		require.Equal(t, 15*time.Second, b.options.ConnMaxIdleTime)
	})
}
