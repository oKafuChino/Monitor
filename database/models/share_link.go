package models

import (
 "time"
 "gorm.io/gorm"
)

// ShareLink stores only a digest; the capability is returned once on creation.
type ShareLink struct {
 ID string `json:"id" gorm:"primaryKey;size:36"`
 TokenHash string `json:"-" gorm:"uniqueIndex;size:64;not null"`
 TokenMask string `json:"token_mask"`
 NodeUUID string `json:"node_uuid" gorm:"index;not null"`
 Duration string `json:"duration"`
 ExpiresAt *time.Time `json:"expires_at"`
 RevokedAt *time.Time `json:"revoked_at"`
 CreatedAt time.Time `json:"created_at"`
 UpdatedAt time.Time `json:"updated_at"`
 CreatedBy string `json:"-"`
}

type ShareSession struct {
 TokenHash string `json:"-" gorm:"primaryKey;size:64"`
 ShareLinkID string `json:"-" gorm:"index;not null"`
 ExpiresAt time.Time `json:"-" gorm:"index;not null"`
}

// RevokeNodeShares runs inside the same transaction as visibility/deletion.
func RevokeNodeShares(tx *gorm.DB, uuid string) error {
 return tx.Model(&ShareLink{}).Where("node_uuid = ? AND revoked_at IS NULL", uuid).Update("revoked_at", time.Now().UTC()).Error
}
