package models

type Repository struct {
	ID             int    `bigquery:"id" json:"id" gorm:"primaryKey,column:id"`
	Name           string `bigquery:"name" json:"name" gorm:"column:name"`
	ReleaseTag     string `bigquery:"releaseTag" json:"releaseTag" gorm:"column:releaseTag"`
	RepositoryHead string `bigquery:"repositoryHead" json:"repositoryHead" gorm:"column:repositoryHead"`
}
