package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"google.golang.org/api/idtoken"
)

var ErrGoogleIDTokenInvalid = errors.New("google id token invalid")
var ErrGoogleIDTokenAudienceInvalid = errors.New("google id token audience invalid")

type GoogleIDTokenClaims struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	Picture       string
	Audience      string
	Issuer        string
}

type GoogleIDTokenVerifier struct {
	clientIDs []string
}

func NewGoogleIDTokenVerifier(clientIDs []string) *GoogleIDTokenVerifier {
	normalized := make([]string, 0, len(clientIDs))
	for _, clientID := range clientIDs {
		clientID = strings.TrimSpace(clientID)
		if clientID != "" {
			normalized = append(normalized, clientID)
		}
	}
	return &GoogleIDTokenVerifier{clientIDs: normalized}
}

func (v *GoogleIDTokenVerifier) Verify(ctx context.Context, rawToken string) (GoogleIDTokenClaims, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return GoogleIDTokenClaims{}, ErrGoogleIDTokenInvalid
	}
	if len(v.clientIDs) == 0 {
		return GoogleIDTokenClaims{}, fmt.Errorf("%w: no google client id configured", ErrGoogleIDTokenInvalid)
	}

	var lastErr error
	for _, clientID := range v.clientIDs {
		payload, err := idtoken.Validate(ctx, rawToken, clientID)
		if err != nil {
			lastErr = err
			continue
		}

		claims := GoogleIDTokenClaims{
			Subject:       payload.Subject,
			Email:         stringClaim(payload.Claims, "email"),
			EmailVerified: boolClaim(payload.Claims, "email_verified"),
			Name:          stringClaim(payload.Claims, "name"),
			Picture:       stringClaim(payload.Claims, "picture"),
			Audience:      payload.Audience,
			Issuer:        payload.Issuer,
		}
		if claims.Subject == "" || claims.Email == "" {
			return GoogleIDTokenClaims{}, fmt.Errorf("%w: missing subject or email", ErrGoogleIDTokenInvalid)
		}
		return claims, nil
	}

	if lastErr != nil {
		if payload, err := idtoken.ParsePayload(rawToken); err == nil && !v.hasClientID(payload.Audience) {
			return GoogleIDTokenClaims{}, fmt.Errorf("%w: %v", ErrGoogleIDTokenAudienceInvalid, lastErr)
		}
		return GoogleIDTokenClaims{}, fmt.Errorf("%w: %v", ErrGoogleIDTokenInvalid, lastErr)
	}
	return GoogleIDTokenClaims{}, ErrGoogleIDTokenInvalid
}

func (v *GoogleIDTokenVerifier) hasClientID(audience string) bool {
	for _, clientID := range v.clientIDs {
		if audience == clientID {
			return true
		}
	}
	return false
}

func stringClaim(claims map[string]any, key string) string {
	value, _ := claims[key].(string)
	return strings.TrimSpace(value)
}

func boolClaim(claims map[string]any, key string) bool {
	value, _ := claims[key].(bool)
	return value
}
