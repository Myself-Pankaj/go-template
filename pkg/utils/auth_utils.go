package utils

import (
	"crypto/rand"
	"go-server/internal/models"
	"math/big"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrEmptyPassword = models.NewAppError(
		models.ErrCodeValidation,
		"password cannot be empty",
		400,
		nil,
	)
)
// GenerateOTP generates a 6-digit OTP
func GenerateOTP() (string, error) {
	const otpLength = 6
	const digits = "0123456789"
	
	otp := make([]byte, otpLength)
	for i := range otp {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", err
		}
		otp[i] = digits[num.Int64()]
	}
	
	return string(otp), nil
}

func HashPassword(password string) (string, error) {
	if password == "" {
		return "", ErrEmptyPassword
	}

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashedBytes), nil
}