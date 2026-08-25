package workqueue

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/api/jobrunscan"
)

// RiverProcess adapts a River client to the DaemonProcess interface so it
// participates in DaemonServer's goroutine lifecycle.
type RiverProcess struct {
	pool        *pgxpool.Pool
	client      *river.Client[pgx.Tx]
	reEvaluator *jobrunscan.ReEvaluator
}

// NewRiverProcess creates a RiverProcess adapter.
func NewRiverProcess(pool *pgxpool.Pool, client *river.Client[pgx.Tx], reEvaluator *jobrunscan.ReEvaluator) *RiverProcess {
	return &RiverProcess{
		pool:        pool,
		client:      client,
		reEvaluator: reEvaluator,
	}
}

// Run starts the River client and blocks until ctx is cancelled. It performs
// an initial symptom cache warm-up (non-fatal on failure since the cache is
// refreshed when the first batch arrives) and a graceful shutdown with a
// 30-second timeout.
func (p *RiverProcess) Run(ctx context.Context) {
	if _, err := p.reEvaluator.RefreshSymptomCache(); err != nil {
		log.WithError(err).Warn("workqueue: initial symptom cache warm-up failed; cache will refresh on first batch")
	}

	if err := p.client.Start(ctx); err != nil {
		log.WithError(err).Error("workqueue: failed to start River client")
		return
	}
	log.Info("workqueue: River client started")

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := p.client.Stop(shutdownCtx); err != nil {
		log.WithError(err).Error("workqueue: error stopping River client")
	}
	p.pool.Close()
	log.Info("workqueue: River client stopped")
}
