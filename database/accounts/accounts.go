package accounts

import (
	"crypto/sha256"
	"encoding/base64"
	"crypto/rand"
	"fmt"
	"crypto/subtle"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"gorm.io/gorm"
	"golang.org/x/crypto/argon2"
	"github.com/komari-monitor/komari/internal/securitylimit"
)

const constantSalt = "06Wm4Jv1Hkxx"
const maxPasswordBytes = 1024
var passwordWork = make(chan struct{}, 4)
var reauthAttempts = securitylimit.New(4096)

func VerifyReauthentication(user models.User, password string) bool {
	if !reauthAttempts.Allow(user.UUID, 10, time.Minute) { return false }
	ok, _ := verifyPassword(password, user.Passwd)
	return ok
}

// CheckPassword 检查密码是否正确
//
// 如果密码正确，返回用户的 UUID 和 true；否则返回空字符串和 false
func CheckPassword(username, passwd string) (uuid string, success bool) {
	db := dbcore.GetDBInstance()
	var user models.User
	result := db.Where("username = ?", username).First(&user)
	if result.Error != nil {
		// 静默处理错误，不显示日志
		return "", false
	}
	if len(passwd) > maxPasswordBytes { return "", false }
	valid, legacy := verifyPassword(passwd, user.Passwd)
	if !valid {
		return "", false
	}
	if legacy {
		encoded := hashPasswd(passwd)
		if encoded == "" { return "", false }
		// Compare-and-swap avoids racing a password reset with hash migration.
		result := db.Model(&models.User{}).Where("uuid = ? AND passwd = ?", user.UUID, user.Passwd).Update("passwd", encoded)
		if result.Error != nil { return "", false }
		if result.RowsAffected != 1 {
			// Another login may have upgraded this row. Recheck the current
			// password so a concurrent reset still invalidates the old one.
			var current models.User
			if db.Where("uuid = ?", user.UUID).First(&current).Error != nil { return "", false }
			if ok, _ := verifyPassword(passwd, current.Passwd); !ok { return "", false }
		}
	}
	return user.UUID, true
}

// ForceResetPassword 强制重置用户密码
func ForceResetPassword(username, passwd string) (err error) {
	if len(passwd) == 0 || len(passwd) > maxPasswordBytes { return fmt.Errorf("密码长度无效") }
	db := dbcore.GetDBInstance()
	encoded := hashPasswd(passwd)
	if encoded == "" { return fmt.Errorf("password hashing unavailable") }
	return db.Transaction(func(tx *gorm.DB) error {
	result := tx.Model(&models.User{}).Where("username = ?", username).Update("passwd", encoded)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("无法找到用户名")
	}
	return tx.Where("1 = 1").Delete(&models.Session{}).Error
	})
}

// hashPasswd 对密码进行加盐哈希
func hashPasswd(passwd string) string {
	if len(passwd) > maxPasswordBytes { return "" }
	select { case passwordWork <- struct{}{}: defer func(){ <-passwordWork }(); default: return "" }
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil { return "" }
	digest := argon2.IDKey([]byte(passwd), salt, 2, 19*1024, 1, 32)
	return fmt.Sprintf("argon2id$v=19$m=19456,t=2,p=1$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(digest))
}

func legacyHashPasswd(passwd string) string {
	saltedPassword := passwd + constantSalt
	hash := sha256.New()
	hash.Write([]byte(saltedPassword))
	hashedPassword := base64.StdEncoding.EncodeToString(hash.Sum(nil))
	return hashedPassword
}

func verifyPassword(passwd, encoded string) (bool, bool) {
	if len(passwd) > maxPasswordBytes { return false, false }
	parts := strings.Split(encoded, "$")
	var salt64, digest64 string
	var m, t, p uint32
	if len(parts) == 5 && parts[0] == "argon2id" && parts[1] == "v=19" && parts[2] == "m=19456,t=2,p=1" {
		m,t,p = 19*1024,2,1; salt64, digest64 = parts[3], parts[4]
	}
	if m == 19*1024 && t == 2 && p == 1 {
		salt, e1 := base64.RawStdEncoding.DecodeString(salt64); expected, e2 := base64.RawStdEncoding.DecodeString(digest64)
		if e1 == nil && e2 == nil && len(salt) == 16 && len(expected) == 32 {
			select { case passwordWork <- struct{}{}: defer func(){ <-passwordWork }(); default: return false, false }
			got := argon2.IDKey([]byte(passwd), salt, t, m, uint8(p), 32); return subtle.ConstantTimeCompare(got, expected)==1, false
		}
	}
	return subtle.ConstantTimeCompare([]byte(legacyHashPasswd(passwd)), []byte(encoded)) == 1, true
}

func CreateAccount(username, passwd string) (user models.User, err error) {
	return CreateAccountWithDB(dbcore.GetDBInstance(), username, passwd)
}

func CreateAccountWithDB(db *gorm.DB, username, passwd string) (user models.User, err error) {
	if len(passwd) == 0 || len(passwd) > maxPasswordBytes { return models.User{}, fmt.Errorf("密码长度无效") }
	hashedPassword := hashPasswd(passwd)
	if hashedPassword == "" { return models.User{}, fmt.Errorf("password hashing unavailable") }
	user = models.User{
		UUID:     uuid.New().String(),
		Username: username,
		Passwd:   hashedPassword,
	}
	err = db.Create(&user).Error
	if err != nil {
		return models.User{}, err
	}
	return user, nil
}

func DeleteAccountByUsername(username string) (err error) {
	return DeleteAccountByUsernameWithDB(dbcore.GetDBInstance(), username)
}

func DeleteAccountByUsernameWithDB(db *gorm.DB, username string) (err error) {
	err = db.Where("username = ?", username).Delete(&models.User{}).Error
	if err != nil {
		return err
	}
	return nil
}

func GetUserByUUID(uuid string) (user models.User, err error) {
	db := dbcore.GetDBInstance()
	err = db.Where("uuid = ?", uuid).First(&user).Error
	if err != nil {
		return models.User{}, err
	}
	return user, nil
}

// 通过 SSO 信息获取用户
func GetUserBySSO(ssoID string) (user models.User, err error) {
	db := dbcore.GetDBInstance()

	// 首先尝试查找已存在的用户
	err = db.Where("sso_id = ?", ssoID).First(&user).Error
	if err == nil {
		return user, nil
	}

	// 如果找不到用户，返回明确的错误信息
	return models.User{}, fmt.Errorf("用户不存在：%s", ssoID)
}

func BindingExternalAccount(uuid string, sso_id string) error {
	db := dbcore.GetDBInstance()
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("uuid = ?", uuid).Update("sso_id", sso_id).Error; err != nil { return err }
		return tx.Where("uuid = ?", uuid).Delete(&models.Session{}).Error
	})
}

func UnbindExternalAccount(uuid string) error {
	db := dbcore.GetDBInstance()
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("uuid = ?", uuid).Update("sso_id", "").Error; err != nil { return err }
		return tx.Where("uuid = ?", uuid).Delete(&models.Session{}).Error
	})
}

func UpdateUser(uuid string, name, password, sso_type *string) error {
	db := dbcore.GetDBInstance()
	// Check if user exists
	var existingUser models.User
	result := db.Where("uuid = ?", uuid).First(&existingUser)
	if result.Error != nil {
		return fmt.Errorf("user not found: %s", uuid)
	}
	updates := make(map[string]interface{})
	if name != nil {
		updates["username"] = *name
	}
	if password != nil {
		if len(*password) == 0 || len(*password) > maxPasswordBytes { return fmt.Errorf("密码长度无效") }
		updates["passwd"] = hashPasswd(*password)
		if updates["passwd"] == "" { return fmt.Errorf("password hashing unavailable") }
	}
	if sso_type != nil {
		updates["sso_type"] = *sso_type
	}
	updates["updated_at"] = time.Now().UTC()
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.User{}).Where("uuid = ?", uuid).Updates(updates).Error; err != nil { return err }
		return tx.Where("uuid = ?", uuid).Delete(&models.Session{}).Error
	})
}
