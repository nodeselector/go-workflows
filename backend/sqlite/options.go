package sqlite

import (
	"strings"
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

	// AutoVacuum runs `PRAGMA auto_vacuum=<status>` when creating the connection to enable the sqlite auto-vacuum feature.
	//
	// If AutoVacuum is set to full or incremental for an existing database, clients may wish to also set
	// VacuumOnStart=true to ensure auto-vacuum is correctly enabled.
	//
	// See
	// - https://sqlite.org/pragma.html#pragma_auto_vacuum
	// - https://sqlite.org/lang_vacuum.html.
	AutoVacuum string

	// VacuumOnStart runs `VACUUM;` when creating the backend database connection.
	VacuumOnStart bool

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

// WithAutoVacuum sets sqlite auto-vacuum to the provided status. See options.AutoVacuum for details.
func WithAutoVacuum(status string) option {
	return func(o *options) {
		status = strings.ToLower(status)

		switch status {
		case "0", "none", "1", "full", "2", "incremental":
			o.AutoVacuum = status
		default:
			o.AutoVacuum = ""
		}
	}
}

// WithFullAutoVacuum sets sqlite auto-vacuum to full. See options.AutoVacuum for details.
func WithFullAutoVacuum() option {
	return WithAutoVacuum("full")
}

// WithIncrementalAutoVacuum sets sqlite auto-vacuum to incremental. See options.AutoVacuum for details.
func WithIncrementalAutoVacuum() option {
	return WithAutoVacuum("incremental")
}

// WithVacuumOnStart sets options.VacuumOnStart=true.
func WithVacuumOnStart() option {
	return func(o *options) {
		o.VacuumOnStart = true
	}
}
