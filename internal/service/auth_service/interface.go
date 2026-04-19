package authservice

import "context"

// EmailService defines the email operations required within this package.
// This must stay in sync with go-server/internal/service.EmailService.
type EmailService interface {
	SendOTP(ctx context.Context, to, otp, name string) error
	SendWelcomeEmail(ctx context.Context, to, name string) error
	SendPasswordResetEmail(ctx context.Context, to, resetLink, name string) error
	SendForgetPasswordEmail(ctx context.Context, to, otp, name string) error
	Close() error
}