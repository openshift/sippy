package models

import (
	"time"
)

type JobRun struct {
	ID             int       `bigquery:"id" json:"id" gorm:"primaryKey,column:id"`
	ReleaseTag     string    `bigquery:"releaseTag" json:"releaseTag" gorm:"column:releaseTag"`
	Name           string    `bigquery:"name" json:"name" gorm:"column:name"`
	JobName        string    `bigquery:"jobName" json:"jobName" gorm:"column:jobName"`
	Kind           string    `bigquery:"kind" json:"kind" gorm:"column:kind"`
	State          string    `bigquery:"state" json:"state" gorm:"column:state"`
	TransitionTime time.Time `bigquery:"transitionTime" gorm:"column:transitionTime" json:"transitionTime"`
	Retries        int64     `bigquery:"retries" gorm:"column:retries" json:"retries"`
	URL            string    `bigquery:"url" json:"url" gorm:"column:url"`
	UpgradesFrom   string    `bigquery:"upgradesFrom" json:"upgradesFrom" gorm:"column:upgradesFrom"`
	UpgradesTo     string    `bigquery:"upgradesTo" json:"upgradesTo" gorm:"column:upgradesTo"`
	Upgrade        bool      `bigquery:"upgrade" json:"upgrade" gorm:"column:upgrade"`
}
