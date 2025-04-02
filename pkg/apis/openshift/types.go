package openshift

type ClusterOperatorList struct {
	APIVersion string            `json:"apiVersion"`
	Items      []ClusterOperator `json:"items"`
}

type ClusterOperator struct {
	APIVersion string         `json:"apiVersion"`
	Kind       string         `json:"kind"`
	Metadata   Metadata       `json:"metadata"`
	Spec       map[string]any `json:"spec"`
	Status     ClusterStatus  `json:"status"`
}

type Metadata struct {
	Name string `json:"name"`
}

type ClusterStatus struct {
	Conditions []Condition `json:"conditions"`
}

type Condition struct {
	LastTransitionTime string `json:"lastTransitionTime"`
	Message            string `json:"message"`
	Reason             string `json:"reason"`
	Status             string `json:"status"`
	Type               string `json:"type"`
}
