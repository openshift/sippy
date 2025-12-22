package models

import (
	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/civil"
)

type JobRunLabel struct {
	ID        string              `bigquery:"prowjob_build_id"`
	StartTime civil.DateTime      `bigquery:"prowjob_start"`
	Label     string              `bigquery:"label"`
	Comment   string              `bigquery:"comment"`
	User      bigquery.NullString `bigquery:"user"`
	Url       string              `bigquery:"-"`
}
