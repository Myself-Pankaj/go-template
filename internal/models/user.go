package models

import (
	"time"
)

type User struct {
	ID             int64      `json:"id" db:"id"`
	Name           string     `json:"name" db:"name"`
	Email          string     `json:"email" db:"email"`
	PhoneNumber    string    `json:"phone_number" db:"phone_number"`
	PasswordHash   string     `json:"-" db:"password_hash"`
	Role           string     `json:"role" db:"role"`
	Subscribed     bool       `json:"subscribed" db:"subscribed"`
	PlanType       string     `json:"plan_type" db:"plan_type"`
	SubscriptionID *string    `json:"subscription_id,omitempty" db:"subscription_id"`
	IsVerified     bool       `json:"is_verified" db:"is_verified"`
	TrialStart     time.Time  `json:"trial_start" db:"trial_start"`
	TrialEnd       time.Time  `json:"trial_end" db:"trial_end"`
	LastLogin      *time.Time `json:"last_login,omitempty" db:"last_login"`
	CreatedAt      time.Time  `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at" db:"updated_at"`
}


// EmailVerification stores verification tokens
type EmailVerification struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
	IsUsed    bool      `json:"is_used"`
}


type UserResponse struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	Email          string     `json:"email"`
	PhoneNumber    string     `json:"phone_number"`
	Role           string     `json:"role"`
	Subscribed     bool       `json:"subscribed"`
	PlanType       string     `json:"plan_type"`
	SubscriptionID *string    `json:"subscription_id,omitempty"`
	IsVerified     bool       `json:"is_verified"`
	TrialStart     time.Time  `json:"trial_start"`
	TrialEnd       time.Time  `json:"trial_end"`
	LastLogin      *time.Time `json:"last_login,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

func (u *User) ToResponse() *UserResponse {
	return &UserResponse{
		ID:             u.ID,
		Name:           u.Name,
		Email:          u.Email,
		PhoneNumber:    u.PhoneNumber,
		Role:           u.Role,
		Subscribed:     u.Subscribed,
		PlanType:       u.PlanType,
		SubscriptionID: u.SubscriptionID,
		IsVerified:     u.IsVerified,
		TrialStart:     u.TrialStart,
		TrialEnd:       u.TrialEnd,
		LastLogin:      u.LastLogin,
		CreatedAt:      u.CreatedAt,
	}
}