package sippyserver

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

type DaemonProcess interface {
	Run(ctx context.Context)
}

func NewDaemonServer(processes []DaemonProcess) *DaemonServer {
	da := &DaemonServer{}

	for _, p := range processes {
		da.addProcess(p)
	}

	return da
}

type DaemonServer struct {
	processes []DaemonProcess
}

func (da *DaemonServer) addProcess(process DaemonProcess) {
	if da.processes == nil {
		da.processes = make([]DaemonProcess, 0)
	}

	da.processes = append(da.processes, process)
}

func (da *DaemonServer) Serve() {

	if len(da.processes) < 1 {
		log.Error("Empty process list, exiting")
		return
	}

	log.Info("Started serving")

	pendingContexts := make([]context.CancelFunc, 0)
	wg := sync.WaitGroup{}

	for _, process := range da.processes {
		ctx := context.Background()
		ctx, cancel := context.WithCancel(ctx)

		pendingContexts = append(pendingContexts, cancel)
		wg.Add(1)
		p := process
		go func() {
			defer wg.Done()
			p.Run(ctx)
		}()
	}

	sigChannel := make(chan os.Signal, 1)
	signal.Notify(sigChannel, syscall.SIGINT, syscall.SIGTERM)
	s := <-sigChannel

	log.Infof("Received shutdown signal: %v", s)

	for _, cancel := range pendingContexts {
		log.Info("Canceling context")
		cancel()
	}

	// give them time to finish
	wchan := make(chan struct{})
	go func() {
		defer close(wchan)
		wg.Wait()
	}()

	select {

	case <-wchan:
		log.Info("Wait group completed")
	case <-time.After(10 * time.Second):
		log.Info("Timed out on wait group")
	}

	log.Info("Ended serving ")
}
