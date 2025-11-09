package service

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/smtp"
	"sync"
	"time"

	"go-server/internal/config"
)

//go:embed templates/*.html
var templateFS embed.FS

// EmailService handles all email operations
type EmailService interface {
	SendOTP(ctx context.Context, to, otp, name string) error
	SendWelcomeEmail(ctx context.Context, to, name string) error
	SendPasswordResetEmail(ctx context.Context, to, resetLink, name string) error
	Close() error
}

type emailService struct {
	config    *emailConfig
	templates *emailTemplates
	pool      *smtpPool
}

type emailConfig struct {
	smtpHost     string
	smtpPort     string
	smtpUsername string
	smtpPassword string
	fromEmail    string
	fromName     string
}

type emailTemplates struct {
	otp           *template.Template
	welcome       *template.Template
	passwordReset *template.Template
}

type smtpPool struct {
	mu          sync.Mutex
	connections chan *smtpConnection
	maxConns    int
	timeout     time.Duration
	host        string
	port        string
	auth        smtp.Auth
	closed      bool
}

type smtpConnection struct {
	client    *smtp.Client
}

// NewEmailService creates a new email service instance
func NewEmailService(cfg *config.Config) (EmailService, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, fmt.Errorf("invalid email configuration: %w", err)
	}

	emailCfg := &emailConfig{
		smtpHost:     cfg.SMTPHost,
		smtpPort:     fmt.Sprintf("%d", cfg.SMTPPort),
		smtpUsername: cfg.SMTPUser,
		smtpPassword: cfg.SMTPPassword,
		fromEmail:    cfg.SMTPFrom,
		fromName:     cfg.AppName,
	}

	templates, err := loadTemplates()
	if err != nil {
		return nil, fmt.Errorf("failed to load email templates: %w", err)
	}

	pool := newSMTPPool(emailCfg, 5, 30*time.Second)

	return &emailService{
		config:    emailCfg,
		templates: templates,
		pool:      pool,
	}, nil
}

// validateConfig ensures all required email configuration is present
func validateConfig(cfg *config.Config) error {
	if cfg.SMTPHost == "" {
		return fmt.Errorf("SMTP host is required")
	}
	if cfg.SMTPPort == 0 {
		return fmt.Errorf("SMTP port is required")
	}
	if cfg.SMTPUser == "" {
		return fmt.Errorf("SMTP username is required")
	}
	if cfg.SMTPPassword == "" {
		return fmt.Errorf("SMTP password is required")
	}
	if cfg.SMTPFrom == "" {
		return fmt.Errorf("SMTP from email is required")
	}
	return nil
}

// newSMTPPool creates a new SMTP connection pool
func newSMTPPool(cfg *emailConfig, maxConns int, timeout time.Duration) *smtpPool {
	return &smtpPool{
		connections: make(chan *smtpConnection, maxConns),
		maxConns:    maxConns,
		timeout:     timeout,
		host:        cfg.smtpHost,
		port:        cfg.smtpPort,
		auth:        smtp.PlainAuth("", cfg.smtpUsername, cfg.smtpPassword, cfg.smtpHost),
	}
}

// SendOTP sends an OTP verification email
func (s *emailService) SendOTP(ctx context.Context, to, otp, name string) error {
	data := struct {
		Name string
		OTP  string
		Year int
	}{
		Name: name,
		OTP:  otp,
		Year: time.Now().Year(),
	}

	body, err := s.renderTemplate(s.templates.otp, data)
	if err != nil {
		return fmt.Errorf("failed to render OTP template: %w", err)
	}

	return s.sendEmail(ctx, to, "Your Verification Code", body)
}

// SendWelcomeEmail sends a welcome email after successful verification
func (s *emailService) SendWelcomeEmail(ctx context.Context, to, name string) error {
	data := struct {
		Name string
		Year int
	}{
		Name: name,
		Year: time.Now().Year(),
	}

	body, err := s.renderTemplate(s.templates.welcome, data)
	if err != nil {
		return fmt.Errorf("failed to render welcome template: %w", err)
	}

	return s.sendEmail(ctx, to, "Welcome to Our Platform!", body)
}

// SendPasswordResetEmail sends a password reset email
func (s *emailService) SendPasswordResetEmail(ctx context.Context, to, resetLink, name string) error {
	data := struct {
		Name      string
		ResetLink string
		Year      int
	}{
		Name:      name,
		ResetLink: resetLink,
		Year:      time.Now().Year(),
	}

	body, err := s.renderTemplate(s.templates.passwordReset, data)
	if err != nil {
		return fmt.Errorf("failed to render password reset template: %w", err)
	}

	return s.sendEmail(ctx, to, "Reset Your Password", body)
}

// sendEmail sends an email with retry logic and context support
func (s *emailService) sendEmail(ctx context.Context, to, subject, body string) error {
	const maxRetries = 3
	var lastErr error

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("email sending cancelled: %w", ctx.Err())
			case <-time.After(time.Second * time.Duration(attempt)):
			}
		}

		if err := s.sendEmailAttempt(ctx, to, subject, body); err == nil {
			return nil
		} else {
			lastErr = err
		}
	}

	return fmt.Errorf("failed to send email after %d attempts: %w", maxRetries, lastErr)
}

// sendEmailAttempt performs a single email send attempt
func (s *emailService) sendEmailAttempt(ctx context.Context, to, subject, body string) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	msg := s.composeMessage(to, subject, body)
	addr := fmt.Sprintf("%s:%s", s.config.smtpHost, s.config.smtpPort)

	errCh := make(chan error, 1)
	go func() {
		errCh <- smtp.SendMail(addr, s.pool.auth, s.config.fromEmail, []string{to}, msg)
	}()

	select {
	case <-ctx.Done():
		return fmt.Errorf("email send timeout: %w", ctx.Err())
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("smtp send failed: %w", err)
		}
		return nil
	}
}

// composeMessage creates the email message with proper headers
func (s *emailService) composeMessage(to, subject, body string) []byte {
	headers := map[string]string{
		"From":         fmt.Sprintf("%s <%s>", s.config.fromName, s.config.fromEmail),
		"To":           to,
		"Subject":      subject,
		"MIME-Version": "1.0",
		"Content-Type": "text/html; charset=UTF-8",
		"Date":         time.Now().Format(time.RFC1123Z),
	}

	var msg bytes.Buffer
	for k, v := range headers {
		msg.WriteString(fmt.Sprintf("%s: %s\r\n", k, v))
	}
	msg.WriteString("\r\n")
	msg.WriteString(body)

	return msg.Bytes()
}

// renderTemplate renders an email template with data
func (s *emailService) renderTemplate(tmpl *template.Template, data interface{}) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("template execution failed: %w", err)
	}
	return buf.String(), nil
}

// loadTemplates loads all email templates from embedded filesystem
func loadTemplates() (*emailTemplates, error) {
	otp, err := template.ParseFS(templateFS, "templates/otp_email.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse OTP template: %w", err)
	}

	welcome, err := template.ParseFS(templateFS, "templates/welcome_email.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse welcome template: %w", err)
	}

	reset, err := template.ParseFS(templateFS, "templates/reset_email.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse password reset template: %w", err)
	}

	return &emailTemplates{
		otp:           otp,
		welcome:       welcome,
		passwordReset: reset,
	}, nil
}

// Close gracefully shuts down the email service
func (s *emailService) Close() error {
	return s.pool.close()
}

// close closes the SMTP connection pool
func (p *smtpPool) close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}

	p.closed = true
	close(p.connections)

	for conn := range p.connections {
		if conn.client != nil {
			conn.client.Quit()
		}
	}

	return nil
}