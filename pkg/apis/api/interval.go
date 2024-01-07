package api

import "time"

// Types from origin monitorapi package

type Condition struct {
	Level string `json:"level"`

	Locator string `json:"locator"`
	Message string `json:"message"`
}

type LocatorType string

const (
	LocatorTypePod               LocatorType = "Pod"
	LocatorTypeContainer         LocatorType = "Container"
	LocatorTypeNode              LocatorType = "Node"
	LocatorTypeAlert             LocatorType = "Alert"
	LocatorTypeClusterOperator   LocatorType = "ClusterOperator"
	LocatorTypeOther             LocatorType = "Other"
	LocatorTypeDisruption        LocatorType = "Disruption"
	LocatorTypeKubeEvent         LocatorType = "KubeEvent"
	LocatorTypeE2ETest           LocatorType = "E2ETest"
	LocatorTypeAPIServerShutdown LocatorType = "APIServerShutdown"
	LocatorTypeClusterVersion    LocatorType = "ClusterVersion"
	LocatorTypeKind              LocatorType = "Kind"
	LocatorTypeCloudMetrics      LocatorType = "CloudMetrics"
)

type LocatorKey string

const (
	LocatorServer             LocatorKey = "server"   // TODO this looks like a bad name.  Aggregated apiserver?  Do we even need it?
	LocatorShutdown           LocatorKey = "shutdown" // TODO this should not exist.  This is a reason and message
	LocatorClusterOperatorKey LocatorKey = "clusteroperator"
	LocatorClusterVersionKey  LocatorKey = "clusterversion"
	LocatorNamespaceKey       LocatorKey = "namespace"
	LocatorDeploymentKey      LocatorKey = "deployment"
	LocatorNodeKey            LocatorKey = "node"
	LocatorEtcdMemberKey      LocatorKey = "etcd-member"
	LocatorKindKey            LocatorKey = "kind"
	LocatorNameKey            LocatorKey = "name"
	LocatorHmsgKey            LocatorKey = "hmsg"
	LocatorPodKey             LocatorKey = "pod"
	LocatorUIDKey             LocatorKey = "uid"
	LocatorMirrorUIDKey       LocatorKey = "mirror-uid"
	LocatorContainerKey       LocatorKey = "container"
	LocatorAlertKey           LocatorKey = "alert"
	LocatorRouteKey           LocatorKey = "route"
	// LocatorBackendDisruptionNameKey holds the value used to store and locate historical data related to the amount of disruption.
	LocatorBackendDisruptionNameKey LocatorKey = "backend-disruption-name"
	LocatorDisruptionKey            LocatorKey = "disruption"
	LocatorE2ETestKey               LocatorKey = "e2e-test"
	LocatorLoadBalancerKey          LocatorKey = "load-balancer"
	LocatorConnectionKey            LocatorKey = "connection"
	LocatorProtocolKey              LocatorKey = "protocol"
	LocatorTargetKey                LocatorKey = "target"
	LocatorRowKey                   LocatorKey = "row"
	LocatorShutdownKey              LocatorKey = "shutdown"
	LocatorServerKey                LocatorKey = "server"
	LocatorMetricKey                LocatorKey = "metric"
)
type Locator struct {
	Type LocatorType `json:"type"`

	// annotations will include the Reason and Cause under their respective keys
	Keys map[LocatorKey]string `json:"keys"`
}

type IntervalReason string

const (
	IPTablesNotPermitted IntervalReason = "iptables-operation-not-permitted"

	DisruptionBeganEventReason              IntervalReason = "DisruptionBegan"
	DisruptionEndedEventReason              IntervalReason = "DisruptionEnded"
	DisruptionSamplerOutageBeganEventReason IntervalReason = "DisruptionSamplerOutageBegan"
	GracefulAPIServerShutdown               IntervalReason = "GracefulShutdownWindow"

	HttpClientConnectionLost IntervalReason = "HttpClientConnectionLost"

	PodPendingReason               IntervalReason = "PodIsPending"
	PodNotPendingReason            IntervalReason = "PodIsNotPending"
	PodReasonCreated               IntervalReason = "Created"
	PodReasonGracefulDeleteStarted IntervalReason = "GracefulDelete"
	PodReasonForceDelete           IntervalReason = "ForceDelete"
	PodReasonDeleted               IntervalReason = "Deleted"
	PodReasonScheduled             IntervalReason = "Scheduled"
	PodReasonEvicted               IntervalReason = "Evicted"
	PodReasonPreempted             IntervalReason = "Preempted"
	PodReasonFailed                IntervalReason = "Failed"

	ContainerReasonContainerExit      IntervalReason = "ContainerExit"
	ContainerReasonContainerStart     IntervalReason = "ContainerStart"
	ContainerReasonContainerWait      IntervalReason = "ContainerWait"
	ContainerReasonReadinessFailed    IntervalReason = "ReadinessFailed"
	ContainerReasonReadinessErrored   IntervalReason = "ReadinessErrored"
	ContainerReasonStartupProbeFailed IntervalReason = "StartupProbeFailed"
	ContainerReasonReady              IntervalReason = "Ready"
	ContainerReasonRestarted          IntervalReason = "Restarted"
	ContainerReasonNotReady           IntervalReason = "NotReady"
	TerminationStateCleared           IntervalReason = "TerminationStateCleared"

	PodReasonDeletedBeforeScheduling IntervalReason = "DeletedBeforeScheduling"
	PodReasonDeletedAfterCompletion  IntervalReason = "DeletedAfterCompletion"

	NodeUpdateReason   IntervalReason = "NodeUpdate"
	NodeNotReadyReason IntervalReason = "NotReady"
	NodeFailedLease    IntervalReason = "FailedToUpdateLease"

	MachineConfigChangeReason  IntervalReason = "MachineConfigChange"
	MachineConfigReachedReason IntervalReason = "MachineConfigReached"

	Timeout IntervalReason = "Timeout"

	E2ETestStarted  IntervalReason = "E2ETestStarted"
	E2ETestFinished IntervalReason = "E2ETestFinished"

	CloudMetricsExtrenuous                IntervalReason = "CloudMetricsExtrenuous"
	FailedToDeleteCGroupsPath             IntervalReason = "FailedToDeleteCGroupsPath"
	FailedToAuthenticateWithOpenShiftUser IntervalReason = "FailedToAuthenticateWithOpenShiftUser"
)

type AnnotationKey string

const (
	AnnotationAlertState         AnnotationKey = "alertstate"
	AnnotationState              AnnotationKey = "state"
	AnnotationSeverity           AnnotationKey = "severity"
	AnnotationReason             AnnotationKey = "reason"
	AnnotationContainerExitCode  AnnotationKey = "code"
	AnnotationCause              AnnotationKey = "cause"
	AnnotationConfig             AnnotationKey = "config"
	AnnotationContainer          AnnotationKey = "container"
	AnnotationImage              AnnotationKey = "image"
	AnnotationInteresting        AnnotationKey = "interesting"
	AnnotationCount              AnnotationKey = "count"
	AnnotationNode               AnnotationKey = "node"
	AnnotationEtcdLocalMember    AnnotationKey = "local-member-id"
	AnnotationEtcdTerm           AnnotationKey = "term"
	AnnotationEtcdLeader         AnnotationKey = "leader"
	AnnotationPreviousEtcdLeader AnnotationKey = "prev-leader"
	AnnotationPathological       AnnotationKey = "pathological"
	AnnotationConstructed        AnnotationKey = "constructed"
	AnnotationPhase              AnnotationKey = "phase"
	AnnotationIsStaticPod        AnnotationKey = "mirrored"
	// TODO this looks wrong. seems like it ought to be set in the to/from
	AnnotationDuration       AnnotationKey = "duration"
	AnnotationRequestAuditID AnnotationKey = "request-audit-id"
	AnnotationRoles          AnnotationKey = "roles"
	AnnotationStatus         AnnotationKey = "status"
	AnnotationCondition      AnnotationKey = "condition"
)
type Message struct {
	// TODO: reason/cause both fields and annotations...
	Reason       IntervalReason `json:"reason"`
	Cause        string         `json:"cause"`
	HumanMessage string         `json:"humanMessage"`

	// annotations will include the Reason and Cause under their respective keys
	Annotations map[AnnotationKey]string `json:"annotations"`
}
type EventInterval struct {
	Condition

	Source string `json:"tempSource,omitempty"` // also temporary, unsure if this concept will survive
	// TODO: we're hoping to move these to just locator/message when everything is ready.
	StructuredLocator Locator `json:"tempStructuredLocator"`
	StructuredMessage Message `json:"tempStructuredMessage"`

	From time.Time `json:"from"`
	To   time.Time `json:"to"`
	// Filename is the base filename we read the intervals from in gcs. If multiple,
	// that usually means one for upgrade and one for conformance portions of the job run.
	// TODO: this may need to be revisited once we're further along with the UI/new schema.
	Filename string `json:"filename"`
}

type EventIntervalList struct {
	Items []EventInterval `json:"items"`
}
