package loaderwithmetrics

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/push"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/dataloader"
)

var loadMetric = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "sippy_data_load_millis",
	Help:    "Milliseconds to load data into the DB",
	Buckets: []float64{5000, 10000, 30000, 60000, 300000, 600000, 1200000, 1800000, 2400000, 3000000, 3600000},
}, []string{"loader"})

var errorMetric = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "sippy_data_load_errors",
	Help:    "Errors encountered while trying to load data into the DB",
	Buckets: []float64{0, 1, 10, 100, 1000},
}, []string{"loader"})

type LoaderWithMetrics struct {
	loaders    []dataloader.DataLoader
	promPusher *push.Pusher
}

func New(wrappedLoaders []dataloader.DataLoader) *LoaderWithMetrics {
	loader := &LoaderWithMetrics{
		loaders: wrappedLoaders,
	}

	if pushgateway := os.Getenv("SIPPY_PROMETHEUS_PUSHGATEWAY"); pushgateway != "" {
		loader.promPusher = push.New(pushgateway, "sippy-prow-job-loader")
		loader.promPusher.Collector(errorMetric)
		loader.promPusher.Collector(loadMetric)
	}

	return loader
}

var loaderOrder = []string{"prow", "releases", "jira", "github", "bugs", "test-mapping", "feature-gates", "component-readiness-cache", "regression-tracker"}

// sortLoaders guarantees that the loaders run in a predicable and proper order
// this is mostly necessary to guarantee that "component-readiness-cache" runs prior to "regression-tracker"
func (l *LoaderWithMetrics) sortLoaders() {
	orderIndex := func() map[string]int {
		m := make(map[string]int, len(loaderOrder))
		for i, v := range loaderOrder {
			m[v] = i
		}
		return m
	}()

	getLoaderIndex := func(name string) int {
		if index, exists := orderIndex[name]; exists {
			return index
		}
		// Unknown loaders get placed at the end
		return len(loaderOrder)
	}

	sort.Slice(l.loaders, func(i, j int) bool {
		return getLoaderIndex(l.loaders[i].Name()) < getLoaderIndex(l.loaders[j].Name())
	})
}

func (l *LoaderWithMetrics) Load() {
	l.sortLoaders()
	overallStart := time.Now()
	log.Infof("starting %d loaders...", len(l.loaders))
	for _, loader := range l.loaders {
		log.Infof("starting loader %q with metrics wrapper", loader.Name())
		start := time.Now()
		loader.Load()
		totalTime := time.Since(start)
		log.Infof("loader %q complete after %+v", loader.Name(), totalTime)

		loadMetric.WithLabelValues(loader.Name()).Observe(float64(totalTime.Milliseconds()))
		errorMetric.WithLabelValues(loader.Name()).Observe(float64(len(loader.Errors())))
	}
	overallDuration := time.Since(overallStart)
	log.Infof("%d loaders finished in %+v...", len(l.loaders), overallDuration)
	loadMetric.WithLabelValues("total").Observe(float64(overallDuration.Milliseconds()))

	if l.promPusher != nil {
		log.Info("pushing metrics to prometheus gateway")
		if err := l.promPusher.Add(); err != nil {
			log.WithError(err).Error("could not push to prometheus pushgateway")
		} else {
			log.Info("successfully pushed metrics to prometheus gateway")
		}
	}
}

func (l *LoaderWithMetrics) Errors() []error {
	var errs []error
	for _, loader := range l.loaders {
		for _, err := range loader.Errors() {
			errs = append(errs, errors.Wrap(err, fmt.Sprintf("loader %q returned error", loader.Name())))
		}
	}
	return errs
}
