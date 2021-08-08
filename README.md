# Sippy

<img src=https://raw.github.com/openshift/sippy/master/sippy.svg height=100 width=100>

CIPI (Continuous Integration Private Investigator) aka Sippy.

A tool to process the job results from https://testgrid.k8s.io/

Analyzes any job with a status of `FLAKY` or `FAILING` as reported on the following dashboards:

```
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.6-informing
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.5-informing
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-informing
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.3-informing
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.2-informing
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.6-blocking
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.5-blocking
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-blocking
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.3-blocking
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.2-blocking
```

Reports on which tests fail most frequently along different dimensions:

* overall
* by job
* by platform (e.g. aws, gcp, etc)
* by sig (sig ownership of the test)

Also reports on:
* Job runs that had large groups of test failures in a single run (generally indicative of a fundamental issue rather than a test problem)
* Job pass rates (which jobs are failing frequently, which are not, in sorted order)

Can filter based on time ranges, job names, and various thresholds.  See `./sippy -h`

## Typical usage

```
# Fetch the latest data.  Rerun this periodically to get new data.
$ ./sippy --fetch-data /some/dir --release X.Y
$ ./sippy --server --local-data /some/dir --release X.Y
```

Browse to http://localhost:8080/?release=X.Y to see the report.

To force sippy to reload data from disk (Such as after rerunning fetch data): http://localhost:8080/refresh

## Detailed usage
Sippy can generate custom reports on a per request basis via:

http://localhost:8080/detailed?release=4.5&parm1=foo&param2=bar

Valid parameters include:

* `startDay` - how many days back in history to start looking at job runs

* `endDay` - how many days back in history to stop looking at job runs

* `testSuccessThreshold` - ignore tests that have a passing percentage higher than this value

* `jobFilter` - ignore jobs with names that match this value

* `minTestRuns` - ignore tests that ran fewer than this many times either overall, or within each job or grouping

* `failureClusterThreshold` - minimum number of test failures in a single job run to be considered a failure cluster/grouping

* `jobTestCount` - number of failing tests to report on for each job definition

## Non-OCP usage

Sippy can be pointed at an arbitrary test-grid dashboard with a more limited featureset.
Instead of using `--release`, you use `--dashboard`, and in order to specify a set of variants, you can use `--variant`.
`--dashboard` usage is `--dashboard=<reportName>=<comma,delimited,list of dashboard names>=<optional ocp version if this is for ocp>`.
`--variant` is limited to one of `{ocp,kube,none}` at the moment, but is expected to allow multiples eventually.

Typical usage for this use-case is like
```
./sippy --fetch-data ../data-dir/  --dashboard=kube-master=sig-release-master-blocking,sig-release-master-informing=
./sippy --server --local-data ../data-dir --dashboard=kube-master=sig-release-master-blocking,sig-release-master-informing= --variant=kube
```

### API

See [the API documentation](pkg/api/README.md)