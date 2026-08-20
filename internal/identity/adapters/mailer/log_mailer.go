package mailer

import (
	"context"
	"fmt"
	"net/url"

	"github.com/ZheglY/family-tree-identity-service/internal/identity/domain"
	"go.uber.org/zap"
)

type LogMailer struct {
	log              *zap.Logger
	verificationURL  *url.URL
	passwordResetURL *url.URL
}

func NewLogMailer(
	log *zap.Logger,
	verificationURL string,
	passwordResetURL string,
) (*LogMailer, error) {
	parsedVerificationURL, err := parsePublicURL(verificationURL)
	if err != nil {
		return nil, fmt.Errorf("invalid email verification URL")
	}
	parsedPasswordResetURL, err := parsePublicURL(passwordResetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid password reset URL")
	}

	return &LogMailer{
		log:              log.Named("development_mailer"),
		verificationURL:  parsedVerificationURL,
		passwordResetURL: parsedPasswordResetURL,
	}, nil
}

func (m *LogMailer) SendVerification(
	_ context.Context,
	email domain.Email,
	token string,
) error {
	verificationURL := withToken(m.verificationURL, token)

	// This adapter is wired only outside production. The token is intentionally
	// visible to make local email verification possible without an SMTP service.
	m.log.Warn(
		"development email verification link",
		zap.String("email", email.String()),
		zap.String("verification_url", verificationURL),
	)

	return nil
}

func (m *LogMailer) SendPasswordReset(
	_ context.Context,
	email domain.Email,
	token string,
) error {
	passwordResetURL := withToken(m.passwordResetURL, token)

	// This adapter is wired only outside production. The token is intentionally
	// visible to make local password recovery possible without an SMTP service.
	m.log.Warn(
		"development password reset link",
		zap.String("email", email.String()),
		zap.String("password_reset_url", passwordResetURL),
	)

	return nil
}

func parsePublicURL(value string) (*url.URL, error) {
	parsedURL, err := url.Parse(value)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("URL must be absolute")
	}
	return parsedURL, nil
}

func withToken(baseURL *url.URL, token string) string {
	result := *baseURL
	query := result.Query()
	query.Set("token", token)
	result.RawQuery = query.Encode()
	return result.String()
}
