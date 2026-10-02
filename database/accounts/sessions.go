package accounts

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"github.com/komari-monitor/komari/utils"
)

// GetAllSessions 获取所有会话
func GetAllSessions() (sessions []models.Session, err error) {
	db := dbcore.GetDBInstance()
	err = db.Find(&sessions).Error
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

// CreateSession 创建新会话
func CreateSession(uuid string, expires int, userAgent, ip, login_method string) (string, error) {
	db := dbcore.GetDBInstance()
	session := utils.GenerateRandomString(32)

	sessionRecord := models.Session{
		UUID:         uuid,
		Session:      session,
		Expires:      time.Now().UTC().Add(time.Duration(expires) * time.Second),
		UserAgent:    userAgent,
		Ip:           ip,
		LoginMethod:  login_method,
		LatestOnline: time.Now().UTC(),
	}
	err := db.Create(&sessionRecord).Error
	if err != nil {
		return "", err
	}
	return session, nil
}

// GetSession 根据会话 ID 获取 UUID
func GetSession(session string) (uuid string, err error) {
	db := dbcore.GetDBInstance()
	var sessionRecord models.Session
	err = db.Where("session = ?", session).First(&sessionRecord).Error
	if err != nil {
		return "", err
	}

	if time.Now().UTC().After(sessionRecord.Expires) {
		// 会话已过期，删除它
		_ = DeleteSession(session)
		return "", errors.New("session expired")
	}

	return sessionRecord.UUID, nil
}

func GetUserBySession(session string) (models.User, error) {
	uuid, err := GetSession(session)
	if err != nil {
		return models.User{}, err
	}
	return GetUserByUUID(uuid)
}

// DeleteSession 删除指定会话
func DeleteSession(session string) (err error) {
	db := dbcore.GetDBInstance()
	result := db.Where("session = ?", session).Delete(&models.Session{})
	if result.Error != nil {
		return result.Error
	}
	return nil
}

// DeleteSessionMasked revokes a session using the short display token returned
// by the admin session list, without exposing the reusable bearer value.
func DeleteSessionMasked(masked string) error {
	var sessions []models.Session
	if err := dbcore.GetDBInstance().Find(&sessions).Error; err != nil { return err }
	for _, item := range sessions {
		if SessionDisplayID(item.Session) == masked { return DeleteSession(item.Session) }
	}
	return errors.New("session not found")
}

func SessionDisplayID(token string) string { digest := sha256.Sum256([]byte(token)); return hex.EncodeToString(digest[:]) }

func DeleteAllSessions() error {
	db := dbcore.GetDBInstance()
	result := db.Where("1 = 1").Delete(&models.Session{})
	if result.Error != nil {
		return result.Error
	}
	return nil
}

func UpdateLatest(session, useragent, ip string) error {
	db := dbcore.GetDBInstance()
	return db.Model(&models.Session{}).Where("session = ?", session).Updates(map[string]interface{}{
		"latest_online":     time.Now().UTC(),
		"latest_user_agent": useragent,
		"latest_ip":         ip,
	}).Error
}

func RemoveExpiredSessions() error {
	db := dbcore.GetDBInstance()
	result := db.Where("expires < ?", time.Now().UTC()).Delete(&models.Session{})
	if result.Error != nil {
		return result.Error
	}
	return nil
}
