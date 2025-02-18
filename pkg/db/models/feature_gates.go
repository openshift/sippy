package models

// FeatureGate maps feature gates to feature sets for a specific release.
//
//	feature gate = NetworkSegmentation
//	feature set  = Default
//	topology     = SelfManagedHA
//	release      = 4.19
//	status       = enabled
type FeatureGate struct {
	*Model
	Release     string `gorm:"column:release;not null;primaryKey;index" json:"release"`
	Topology    string `gorm:"column:topology;not null;primaryKey;index" json:"topology"`
	FeatureSet  string `gorm:"column:feature_set;not null;primaryKey;index" json:"feature_set"`
	FeatureGate string `gorm:"column:feature_gate;not null;primaryKey;index" json:"feature_gate"`
	Status      string `gorm:"column:status;not null" json:"status"`
}
