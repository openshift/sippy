package workqueue

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	log "github.com/sirupsen/logrus"
)

const shutdownTimeout = 30 * time.Second

// RiverProcess adapts a River client to the DaemonProcess interface.
type RiverProcess struct {
	client *river.Client[pgx.Tx]
}

// NewRiverProcess wraps a River client as a DaemonProcess.
func NewRiverProcess(client *river.Client[pgx.Tx]) *RiverProcess {
	return &RiverProcess{client: client}
}

// Run starts the River client and blocks until the context is cancelled.
func (p *RiverProcess) Run(ctx context.Context) {
	if err := p.client.Start(ctx); err != nil {
		log.WithError(err).Error("failed to start River client")
		return
	}
	log.Info("River work queue started")

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := p.client.Stop(shutdownCtx); err != nil {
		log.WithError(err).Error("error stopping River client")
	} else {
		log.Info("River work queue stopped gracefully")
	}
}
