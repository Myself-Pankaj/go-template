package models

import "time"

// ==================== USER ====================

// User represents an authenticated user stored in the database.
// SocietyID is nil for system-level super admins not tied to any society,
// and for newly registered users before they create or join a society.
type User struct {
	ID           int64      `json:"id"                   db:"id"`
	Name         string     `json:"name"                 db:"name"`
	Email        string     `json:"email"                db:"email"`
	PhoneNumber  string     `json:"phone_number"         db:"phone_number"`
	PasswordHash string     `json:"-"                    db:"password_hash"`
	Role         string     `json:"role"                 db:"role"`
	SocietyID    *int64     `json:"society_id,omitempty" db:"society_id"` // nil until assigned
	FlatID       *int64     `json:"flat_id,omitempty"    db:"flat_id"`    // nil unless resident
	IsVerified   bool       `json:"is_verified"          db:"is_verified"`
	LastLogin    *time.Time `json:"last_login,omitempty" db:"last_login"`
	CreatedAt    time.Time  `json:"created_at"           db:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"           db:"updated_at"`
}

// ToResponse returns a safe outbound DTO — never includes PasswordHash.
func (u *User) ToResponse() *UserResponse {
	return &UserResponse{
		ID:          u.ID,
		Name:        u.Name,
		Email:       u.Email,
		PhoneNumber: u.PhoneNumber,
		Role:        u.Role,
		SocietyID:   u.SocietyID,
		FlatID:      u.FlatID,
		IsVerified:  u.IsVerified,
		LastLogin:   u.LastLogin,
		CreatedAt:   u.CreatedAt,
	}
}

// UserResponse is the safe outbound DTO — PasswordHash is intentionally absent.
type UserResponse struct {
	ID          int64      `json:"id"`
	Name        string     `json:"name"`
	Email       string     `json:"email"`
	PhoneNumber string     `json:"phone_number"`
	Role        string     `json:"role"`
	SocietyID   *int64     `json:"society_id,omitempty"`
	FlatID      *int64     `json:"flat_id,omitempty"`
	IsVerified  bool       `json:"is_verified"`
	LastLogin   *time.Time `json:"last_login,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// ==================== EMAIL VERIFICATION ====================

// EmailVerification stores OTP / token records for email confirmation flows.
type EmailVerification struct {
	ID        int64     `json:"id"         db:"id"`
	UserID    int64     `json:"user_id"    db:"user_id"`
	Token     string    `json:"token"      db:"token"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	IsUsed    bool      `json:"is_used"    db:"is_used"`
}