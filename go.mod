module github.com/openshift/sippy

go 1.16

require (
	cloud.google.com/go/bigquery v1.8.0
	github.com/elastic/go-elasticsearch/v7 v7.5.1-0.20210823155509-845c8efe54a7
	github.com/lib/pq v1.10.2
	github.com/montanaflynn/stats v0.6.6
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.11.0
	github.com/spf13/cobra v0.0.6
	github.com/stretchr/testify v1.7.0
	github.com/tidwall/gjson v1.9.4
	golang.org/x/sys v0.0.0-20211109184856-51b60fd695b3 // indirect
	google.golang.org/api v0.60.0
	gorm.io/driver/postgres v1.2.1
	gorm.io/gorm v1.22.2
	k8s.io/klog v1.0.0
)
