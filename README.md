# Sippy

<img src=https://raw.github.com/bparees/sippy/master/sippy.svg height=100 width=100>

CIPI (Continuous Integration Private Investigator) aka Sippy.

A tool to process the job results from https://testgrid.k8s.io/

Analyzes any job with a status of `FLAKY` or `FAILING` as reported on the following dashboards:

```
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.5-informing
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-informing
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.3-informing
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.2-informing
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.1-informing
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.5-blocking
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.4-blocking
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.3-blocking
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.2-blocking
https://testgrid.k8s.io/redhat-openshift-ocp-release-4.1-blocking
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
