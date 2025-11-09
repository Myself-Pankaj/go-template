package models

import "time"

// UserVerification represents OTP verification record
type UserVerification struct {
	ID        int64     `json:"id"`
	Email    string    `json:"email"`
	UserID    int64     `json:"user_id"`
	OTP       string    `json:"otp"`
	IsUsed    bool      `json:"is_used"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

