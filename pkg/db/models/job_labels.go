package models

import (
	"cloud.google.com/go/civil"
)

// JobRunLabel represents a label applied to a job run, stored in BigQuery's job_labels table.
type JobRunLabel struct {
	ID         string         `bigquery:"prowjob_build_id"`
	StartTime  civil.DateTime `bigquery:"prowjob_start"`
	Label      string         `bigquery:"label"`
	Comment    string         `bigquery:"comment"`
	User       string         `bigquery:"user"`
	CreatedAt  civil.DateTime `bigquery:"created_at"`
	UpdatedAt  civil.DateTime `bigquery:"updated_at"`
	SourceTool string         `bigquery:"source_tool"`
	SymptomID  string         `bigquery:"symptom_id"`
	URL        string         `bigquery:"-"`
}

// [2025-12-22] To update the BigQuery table schema, use:
//
//	bq update <project>:<dataset>.job_labels \
//	  created_at:DATETIME,updated_at:DATETIME,source_tool:STRING,symptom_id:STRING
//
// Or via SQL:
//
//	ALTER TABLE `<project>.<dataset>.job_labels`
//	ADD COLUMN IF NOT EXISTS created_at DATETIME,
//	ADD COLUMN IF NOT EXISTS updated_at DATETIME,
//	ADD COLUMN IF NOT EXISTS source_tool STRING,
//	ADD COLUMN IF NOT EXISTS symptom_id STRING;
