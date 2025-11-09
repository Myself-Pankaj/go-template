package service

import "time"

// ==================== CONSTANTS ====================
const (
	DefaultTimeout     = 5 * time.Second
	OTPExpiryDuration  = 5 * time.Minute
	MaxOTPAttempts     = 5
	OTPCooldownPeriod  = 1 * time.Minute
)
