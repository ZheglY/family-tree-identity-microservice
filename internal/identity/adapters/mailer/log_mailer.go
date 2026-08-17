package mailer

import (
	"context"
	"fmt"
	"net/url"

	"github.com/ZheglY/family-tree-identity-service/internal/identity/domain"
	"go.uber.org/zap"
)

type LogMailer struct {
	log             *zap.Logger
	verificationURL *url.URL
}

func NewLogMailer(
	log *zap.Logger,
	verificationURL string,
) (*LogMailer, error) {
	parsedURL, err := url.Parse(verificationURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return nil, fmt.Errorf("invalid email verification URL")
	}

	return &LogMailer{
		log:             log.Named("development_mailer"),
		verificationURL: parsedURL,
	}, nil
}

func (m *LogMailer) SendVerification(
	_ context.Context,
	email domain.Email,
	token string,
) error {
	verificationURL := *m.verificationURL
	query := verificationURL.Query()
	query.Set("token", token)
	verificationURL.RawQuery = query.Encode()

	// This adapter is wired only outside production. The token is intentionally
	// visible to make local email verification possible without an SMTP service.
	m.log.Warn(
		"development email verification link",
		zap.String("email", email.String()),
		zap.String("verification_url", verificationURL.String()),
	)

	return nil
}
