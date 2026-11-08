package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"

	"hephaestus-backend/pkg/database"

	"gorm.io/gorm"
)

// HashPassword returns the same SHA256 password hash used by system login.
func HashPassword(password string) string {
	hash := sha256.Sum256([]byte(password))
	return hex.EncodeToString(hash[:])
}

// VerifyCredentials verifies a username/password pair against the system user table.
func VerifyCredentials(username string, password string) (database.User, bool, error) {
	var user database.User
	if username == "" {
		return user, false, nil
	}

	db, err := database.GetDB()
	if err != nil {
		return user, false, err
	}

	if err := db.Where("username = ?", username).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return user, false, nil
		}
		return user, false, err
	}

	expected := HashPassword(password)
	if subtle.ConstantTimeCompare([]byte(user.Password), []byte(expected)) != 1 {
		return user, false, nil
	}

	return user, true, nil
}
