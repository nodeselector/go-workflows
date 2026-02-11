package main

// drain is a one-shot utility that exercises RemoveWorkflowInstances through
// the full worker/activity machinery — the same code path as production.
// It copies the database first so the original is never modified.
//
// Usage:
//
//	go run ./tools/drain -db /path/to/prod.sqlite
//	go run ./tools/drain -db /path/to/prod.sqlite -dry-run
//	go run ./tools/drain -db /path/to/prod.sqlite -batch 500 -retention 48h
//	go run ./tools/drain -db /path/to/prod.sqlite -activity-timeout 2m

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"time"

	"github.com/cschleiden/go-workflows/backend"
	"github.com/cschleiden/go-workflows/backend/sqlite"
	"github.com/cschleiden/go-workflows/client"
	"github.com/cschleiden/go-workflows/worker"
	"github.com/cschleiden/go-workflows/workflow"

	_ "modernc.org/sqlite"
)

// drainWorkflow calls RemoveWorkflowInstances through the activity framework,
// reproducing the same code path as the production expiration workflow.
// It loops up to maxBatches times, calling one activity per batch.
func drainWorkflow(ctx workflow.Context, params drainParams) (drainResult, error) {
	logger := workflow.Logger(ctx)
	var result drainResult

	for i := 0; i < params.MaxBatches; i++ {
		var a *drainActivities
		_, err := workflow.ExecuteActivity[any](ctx, workflow.ActivityOptions{
			RetryOptions: workflow.RetryOptions{
				MaxAttempts: 1, // No retries — we want to see failures.
			},
		}, a.RemoveExpired, params.Cutoff, params.BatchSize).Get(ctx)
		if err != nil {
			logger.Error("activity failed", "batch", i+1, "error", err)
			return result, err
		}
		result.BatchesRun = i + 1
	}

	return result, nil
}

type drainParams struct {
	Cutoff     time.Time
	BatchSize  int
	MaxBatches int
}

type drainResult struct {
	BatchesRun int
}

type drainActivities struct {
	backend backend.Backend
	logger  *slog.Logger
}

// RemoveExpired is called once per batch, exactly like the production
// expiration activity. It runs under the ActivityLockTimeout.
func (a *drainActivities) RemoveExpired(ctx context.Context, cutoff time.Time, batchSize int) error {
	start := time.Now()
	err := a.backend.RemoveWorkflowInstances(ctx,
		backend.RemoveFinishedBefore(cutoff),
		backend.RemoveFinishedBatchSize(batchSize),
	)
	a.logger.Info("activity removeExpired",
		"elapsed", time.Since(start),
		"error", err,
	)
	return err
}

func main() {
	dbPath := flag.String("db", "", "path to source SQLite database file (will be copied, not modified)")
	batchSize := flag.Int("batch", 100, "batch size per removal call")
	retention := flag.Duration("retention", 24*time.Hour, "retention period — instances completed before now-retention are removed")
	activityTimeout := flag.Duration("activity-timeout", 2*time.Minute, "activity lock timeout (matches production default)")
	dryRun := flag.Bool("dry-run", false, "count expired instances and exit without removing")
	timeout := flag.Duration("timeout", 30*time.Minute, "overall timeout for the drain operation")
	flag.Parse()

	if *dbPath == "" {
		fmt.Fprintln(os.Stderr, "usage: drain -db <path.sqlite> [-batch N] [-retention duration] [-activity-timeout duration] [-dry-run]")
		os.Exit(1)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// ── 1. Copy source database ──────────────────────────────────────────
	copyPath := *dbPath + ".drain-copy"
	logger.Info("copying database", "src", *dbPath, "dst", copyPath)

	if err := copyFile(*dbPath, copyPath); err != nil {
		logger.Error("failed to copy database", "error", err)
		os.Exit(1)
	}
	_ = copyFile(*dbPath+"-wal", copyPath+"-wal")
	_ = copyFile(*dbPath+"-shm", copyPath+"-shm")
	defer func() {
		os.Remove(copyPath)
		os.Remove(copyPath + "-wal")
		os.Remove(copyPath + "-shm")
	}()

	// ── 2. Snapshot ──────────────────────────────────────────────────────
	cutoff := time.Now().Add(-*retention)
	before := snapshot(logger, copyPath, cutoff)

	logger.Info("database snapshot",
		"total_instances", before.total,
		"active", before.active,
		"completed", before.completed,
		"expired_before_cutoff", before.expired,
		"history_rows", before.historyRows,
		"attribute_rows", before.attributeRows,
		"cutoff", cutoff.Format(time.RFC3339),
	)

	if *dryRun || before.expired == 0 {
		if *dryRun {
			logger.Info("dry-run mode, exiting")
		} else {
			logger.Info("nothing to drain")
		}
		return
	}

	// ── 3. Boot worker + client, run drain workflow ──────────────────────
	expectedBatches := int((before.expired + int64(*batchSize) - 1) / int64(*batchSize))
	logger.Info("starting drain through worker machinery",
		"activity_timeout", *activityTimeout,
		"batch_size", *batchSize,
		"expected_batches", expectedBatches,
	)

	b := sqlite.NewSqliteBackend(copyPath,
		sqlite.WithApplyMigrations(false),
		sqlite.WithBackendOptions(
			backend.WithLogger(logger),
			backend.WithActivityLockTimeout(*activityTimeout),
		),
	)
	defer b.Close()

	w := worker.New(b, nil)
	w.RegisterWorkflow(drainWorkflow)
	w.RegisterActivity(&drainActivities{backend: b, logger: logger})

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := w.Start(ctx); err != nil {
		logger.Error("failed to start worker", "error", err)
		os.Exit(1)
	}

	c := client.New(b)

	wallStart := time.Now()
	wfi, err := c.CreateWorkflowInstance(ctx, client.WorkflowInstanceOptions{
		InstanceID: "drain-one-shot",
	}, drainWorkflow, drainParams{
		Cutoff:     cutoff,
		BatchSize:  *batchSize,
		MaxBatches: expectedBatches,
	})
	if err != nil {
		logger.Error("failed to create drain workflow", "error", err)
		os.Exit(1)
	}

	result, err := client.GetWorkflowResult[drainResult](ctx, c, wfi, *timeout)
	wallDur := time.Since(wallStart)

	// Stop worker
	cancel()
	w.WaitForCompletion()
	b.Close()

	if err != nil {
		logger.Error("drain workflow failed",
			"error", err,
			"wall_time", wallDur,
		)
		// Still take a snapshot to see what happened.
		after := snapshot(logger, copyPath, cutoff)
		logger.Info("post-failure snapshot",
			"instances_removed", before.expired-after.expired,
			"history_rows_removed", before.historyRows-after.historyRows,
			"attribute_rows_removed", before.attributeRows-after.attributeRows,
			"instances_remaining", after.expired,
		)
		os.Exit(1)
	}

	// ── 4. Final snapshot ────────────────────────────────────────────────
	after := snapshot(logger, copyPath, cutoff)

	logger.Info("drain complete",
		"wall_time", wallDur,
		"activity_batches", result.BatchesRun,
		"instances_removed", before.expired-after.expired,
		"history_rows_removed", before.historyRows-after.historyRows,
		"attribute_rows_removed", before.attributeRows-after.attributeRows,
		"instances_remaining", after.expired,
	)
}

// ── Database snapshot ────────────────────────────────────────────────────────

type dbStats struct {
	total         int64
	active        int64
	completed     int64
	expired       int64
	historyRows   int64
	attributeRows int64
}

func snapshot(logger *slog.Logger, dbPath string, cutoff time.Time) dbStats {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?mode=ro", dbPath))
	if err != nil {
		logger.Error("failed to open database for snapshot", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	var s dbStats
	db.QueryRow("SELECT COUNT(*) FROM instances").Scan(&s.total)
	db.QueryRow("SELECT COUNT(*) FROM instances WHERE completed_at IS NULL").Scan(&s.active)
	db.QueryRow("SELECT COUNT(*) FROM instances WHERE completed_at IS NOT NULL").Scan(&s.completed)
	db.QueryRow("SELECT COUNT(*) FROM instances WHERE completed_at IS NOT NULL AND completed_at < ?", cutoff).Scan(&s.expired)
	db.QueryRow("SELECT COUNT(*) FROM history").Scan(&s.historyRows)
	db.QueryRow("SELECT COUNT(*) FROM attributes").Scan(&s.attributeRows)
	return s
}

// ── File copy ────────────────────────────────────────────────────────────────

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
