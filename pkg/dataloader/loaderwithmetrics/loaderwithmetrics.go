package loaderwithmetrics

import (
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/push"
	log "github.com/sirupsen/logrus"

	"github.com/openshift/sippy/pkg/dataloader"
)

var prowJobLoadMetric = promauto.NewHistogramVec(prometheus.HistogramOpts{
	Name:    "sippy_data_load_millis",
	Help:    "Milliseconds to load data into the DB",
	Buckets: []float64{5000, 10000, 30000, 60000, 300000, 600000, 1200000, 1800000, 2400000, 3000000, 3600000},
}, []string{"loader"})

type LoaderWithMetrics struct {
	loader     dataloader.DataLoader
	promPusher *push.Pusher
}

func New(wrappedLoader dataloader.DataLoader) *LoaderWithMetrics {
	loader := &LoaderWithMetrics{
		loader: wrappedLoader,
	}

	if pushgateway := os.Getenv("SIPPY_PROMETHEUS_PUSHGATEWAY"); pushgateway != "" {
		loader.promPusher = push.New(pushgateway, "sippy-prow-job-loader")
		loader.promPusher.Collector(prowJobLoadMetric)
	}

	return loader
}

func (l *LoaderWithMetrics) Load() {
	log.Infof("starting loader %q with metrics wrapper", l.loader.Name())
	start := time.Now()
	l.loader.Load()
	totalTime := time.Since(start)
	log.Infof("loader %q complete after %+v", l.loader.Name(), totalTime)

	prowJobLoadMetric.WithLabelValues(l.loader.Name()).Observe(float64(totalTime.Milliseconds()))
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
	return l.loader.Errors()
}
