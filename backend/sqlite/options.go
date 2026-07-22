package sqlite

import (
	"time"

	"github.com/cschleiden/go-workflows/backend"
)

// defaultConnMaxLifetime and defaultConnMaxIdleTime bound how long the
// file-backed pool's single connection may live and sit idle before it is
// recycled. They default to non-zero values so that existing callers pick up
// the self-healing behavior without having to opt in.
const (
	defaultConnMaxLifetime = 5 * time.Minute
	defaultConnMaxIdleTime = 5 * time.Minute
)

type options struct {
	*backend.Options

	// ApplyMigrations automatically applies database migrations on startup.
	ApplyMigrations bool

	// AutoVacuum runs the `PRAGMA auto_vacuum=full` when creating the connection to enable the sqlite auto-vacuum feature.
	//
	// The `VACUUM` statement is always run after enabling auto-vacuum to ensure auto-vacuum is correctly enabled and to
	// reorganize the database file and reclaim disk space.
	//
	// See
	// - https://sqlite.org/pragma.html#pragma_auto_vacuum
	// - https://sqlite.org/lang_vacuum.html.
	AutoVacuum bool

	// ConnMaxLifetime bounds the maximum amount of time a pooled connection may be
	// reused before it is closed and replaced. Because the file-backed pool is
	// capped to a single connection, bounding its lifetime allows the backend to
	// recover from a driver-level fault (such as a failed COMMIT that leaves a
	// transaction dangling) that would otherwise wedge the connection until the
	// process is restarted. A value <= 0 disables connection lifetime recycling.
	ConnMaxLifetime time.Duration

	// ConnMaxIdleTime bounds the maximum amount of time a pooled connection may
	// remain idle before it is closed. Together with ConnMaxLifetime this ensures a
	// tainted connection is eventually discarded so the backend can self-heal. A
	// value <= 0 disables idle-time recycling.
	ConnMaxIdleTime time.Duration
}

type option func(*options)

// WithApplyMigrations automatically applies database migrations on startup.
func WithApplyMigrations(applyMigrations bool) option {
	return func(o *options) {
		o.ApplyMigrations = applyMigrations
	}
}

// WithConnMaxLifetime sets the maximum lifetime of the file-backed pool's
// connection before it is recycled. See options.ConnMaxLifetime for details.
func WithConnMaxLifetime(d time.Duration) option {
	return func(o *options) {
		o.ConnMaxLifetime = d
	}
}

// WithConnMaxIdleTime sets the maximum idle time of the file-backed pool's
// connection before it is recycled. See options.ConnMaxIdleTime for details.
func WithConnMaxIdleTime(d time.Duration) option {
	return func(o *options) {
		o.ConnMaxIdleTime = d
	}
}

// WithBackendOptions allows to pass generic backend options.
func WithBackendOptions(opts ...backend.BackendOption) option {
	return func(o *options) {
		for _, opt := range opts {
			opt(o.Options)
		}
	}
}

// WithAutoVacuum sets sqlite auto-vacuum to full. See options.AutoVacuum for details.
func WithAutoVacuum() option {
	return func(o *options) {
		o.AutoVacuum = true
	}
}
