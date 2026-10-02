package accounts

import (
	"image"
	"fmt"
	"time"

	"github.com/komari-monitor/komari/database/dbcore"
	"github.com/komari-monitor/komari/database/models"
	"github.com/pquerna/otp/totp"
	"github.com/komari-monitor/komari/internal/securitylimit"
	"gorm.io/gorm"
)

var (
	TwoFactorIssuer = "Komari Monitor"
	factorAttempts = securitylimit.New(4096)
)

func Generate2Fa() (string, image.Image, error) {
	otp, err := totp.Generate(totp.GenerateOpts{
		Issuer:      TwoFactorIssuer,
		AccountName: "komari",
	})
	if err != nil {
		return "", nil, err
	}
	img, err := otp.Image(250, 250)
	if err != nil {
		return "", nil, err
	}
	return otp.Secret(), img, nil
}

func Enable2Fa(uuid, secret string) error {
	db := dbcore.GetDBInstance()
	return db.Transaction(func(tx *gorm.DB) error {
	result := tx.Model(&models.User{}).Where("uuid = ? AND (two_factor = '' OR two_factor IS NULL)", uuid).Update("two_factor", secret)
	if result.Error != nil { return result.Error }
	if result.RowsAffected != 1 { return fmt.Errorf("2FA is already enabled") }
	return tx.Where("uuid = ?", uuid).Delete(&models.Session{}).Error
	})
}

func Verify2Fa(uuid, code string) (bool, error) {
	if !factorAttempts.Allow(uuid, 10, time.Minute) { return false, fmt.Errorf("too many verification attempts") }
	db := dbcore.GetDBInstance()
	var user models.User
	err := db.Where("uuid = ?", uuid).First(&user).Error
	if err != nil {
		return false, err
	}

	if user.TwoFactor == "" {
		return false, nil // 用户未启用2FA
	}

	valid := totp.Validate(code, user.TwoFactor)
	if !valid {
		return false, nil
	}

	return true, nil
}

func Disable2Fa(uuid string) error {
	db := dbcore.GetDBInstance()
	return db.Transaction(func(tx *gorm.DB) error {
	if err := tx.Model(&models.User{}).Where("uuid = ?", uuid).Update("two_factor", "").Error; err != nil { return err }
	return tx.Where("uuid = ?", uuid).Delete(&models.Session{}).Error
	})
}
