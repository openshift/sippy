package models

type MetricsRefresh struct {
	Model
	Name string `gorm:"index"`
}
