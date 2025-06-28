package models

import (
	"time"
)

type AuditLog struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	TableName string    `json:"table_name" gorm:"not null"`
	Operation string    `json:"operation" gorm:"not null"`
	RowID     uint      `json:"row_id" gorm:"not null"`
	OldData   []byte    `json:"old_data" gorm:"type:jsonb"`
	NewData   []byte    `json:"new_data" gorm:"type:jsonb"`
	User      string    `json:"user" gorm:"not null"`
	CreatedAt time.Time `json:"created_at" gorm:"autoCreateTime"`
}

type OperationType string

const (
	Create OperationType = "CREATE"
	Update OperationType = "UPDATE"
	Delete OperationType = "DELETE"
)
