package sippyserver

const (
	// Whether this instance of sippy is capable of display OpenShift-releated data. This
	// is basically always since we don't have a kube Sippy anymore.
	OpenshiftCapability = "openshift_releases"

	// LocalDB is whether we have a local DB client (currently postgres)
	LocalDBCapability = "local_db"

	// BuildclusterCapability is whether we have build cluster health data.
	BuildClusterCapability = "build_clusters"

	// ComponentReadiness capability is whether this sippy instance is configured for Component Readiness
	ComponentReadinessCapability = "component_readiness"
)
