package sqlite

import (
	"strings"

	"github.com/cschleiden/go-workflows/backend"
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
}

type option func(*options)

// WithApplyMigrations automatically applies database migrations on startup.
func WithApplyMigrations(applyMigrations bool) option {
	return func(o *options) {
		o.ApplyMigrations = applyMigrations
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
