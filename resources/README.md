# Sippy Sample OpenShift Deployment

This directory contains a sample deployment of Sippy on OpenShift, configured roughly as needed to pull job data from OpenShift CI's Prow instance.

We use OpenShift's BuildConfig/ImageStream/DeploymentConfig as they simplify our prod deployment.

To begin, you'll need a KUBECONFIG pointing to an OpenShift cluster.

## Sippy for OpenShift CI

```bash
oc new-project sippy
```

You will need a GCS credential for reading OpenShift CI artifacts when importing jobs from our prow.


```bash
oc create secret generic gcs-credentials --from-file credentials=$GCS_CRED -n sippy
```

If you want the sippy daemon server and commenting processor to run you need to add a github api token 
You should enable the sippy-github-token in fetchdata-cronjob.yaml as well as --load-github=true for the import
side of things as well

```bash
oc create secret generic sippy-github-token --from-literal token=ghp_THE_TOKEN -n sippy
```

If you do not wish to build and deploy github.com/openshift/sippy main branch, you can edit `resources/buildconfig.yaml` and point to your own fork and branch.

Apply all the manifests:

```bash
oc apply -f resources/openshift/
```

Included is a simple postgresql deployment. In the real world you'll need to edit the postgresql secret to point to your actual database.

You will see a build pod start up, this will take a few minutes to pull source, build, and publish to the integrated OpenShift registry. Once it completes a pre-hook will run to create db schema, and then the sippy pods should start appearing.

The fetchdata CronJob runs once and hour on the hour to populate the db and materialized views, until then sippy will be empty and may not work. You can trigger it manually with:

```bash
oc create job --from=cronjob/fetchdata fetchdata-manual-01
```

The CronJob is configured to just pull one small older release for development purposes, but the others are present just commented out.

We have not included ingress/routing, you will need to expose Sippy yourself, but you can locally access it with:

```bash
oc port-forward svc/sippy 8080:8080
```

This also works if you wish to access the postgres service with a client tool.

## Sippy for Kubernetes CI

```bash
oc new-project sippy
oc apply -f resources/kube/
```



