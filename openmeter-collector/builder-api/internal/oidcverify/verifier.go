package oidcverify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

const defaultRequiredScope = "sign:job"

// VerifiedUser is an end-user identity extracted from an Auth0 access token.
type VerifiedUser struct {
	ClientID       string
	ExternalUserID string
}

// Verifier validates Auth0 end-user access tokens (device code, authorization code).
type Verifier struct {
	issuer   string
	audience string
	cache    *jwk.Cache
	jwksURL  string
}

// New creates a JWKS-cached OIDC access-token verifier.
func New(ctx context.Context, issuer, audience string) (*Verifier, error) {
	issuer = strings.TrimSuffix(strings.TrimSpace(issuer), "/")
	audience = strings.TrimSpace(audience)
	if issuer == "" {
		return nil, fmt.Errorf("oidcverify: issuer is required")
	}
	if audience == "" {
		return nil, fmt.Errorf("oidcverify: audience is required")
	}

	jwksURL := issuer + "/.well-known/jwks.json"
	cache := jwk.NewCache(ctx)
	if err := cache.Register(jwksURL, jwk.WithMinRefreshInterval(15*time.Minute)); err != nil {
		return nil, fmt.Errorf("oidcverify: register jwks: %w", err)
	}

	return &Verifier{
		issuer:   issuer,
		audience: audience,
		cache:    cache,
		jwksURL:  jwksURL,
	}, nil
}

// VerifyUserAccessToken validates a bearer JWT and returns tenant + end-user ids.
func (v *Verifier) VerifyUserAccessToken(ctx context.Context, token, expectedClientID string) (*VerifiedUser, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("missing access token")
	}
	if strings.Count(token, ".") != 2 {
		return nil, fmt.Errorf("not a JWT")
	}

	keySet, err := v.cache.Get(ctx, v.jwksURL)
	if err != nil {
		return nil, fmt.Errorf("load jwks: %w", err)
	}

	parsed, err := jwt.ParseString(
		token,
		jwt.WithKeySet(keySet),
		jwt.WithAudience(v.audience),
	)
	if err != nil {
		return nil, fmt.Errorf("jwt verification failed: %w", err)
	}
	if normalizeIssuer(parsed.Issuer()) != normalizeIssuer(v.issuer) {
		return nil, fmt.Errorf("jwt verification failed: iss not satisfied")
	}

	if err := requireScope(parsed, defaultRequiredScope); err != nil {
		return nil, err
	}

	clientID := claimString(parsed, "azp")
	if clientID == "" {
		return nil, fmt.Errorf("token missing azp claim")
	}
	expectedClientID = strings.TrimSpace(expectedClientID)
	if expectedClientID != "" && clientID != expectedClientID {
		return nil, fmt.Errorf("token azp does not match clientId")
	}

	externalUserID := claimString(parsed, "external_user_id")
	if externalUserID == "" {
		externalUserID = parsed.Subject()
	}
	if externalUserID == "" {
		return nil, fmt.Errorf("token missing sub claim")
	}

	return &VerifiedUser{
		ClientID:       clientID,
		ExternalUserID: externalUserID,
	}, nil
}

func requireScope(tok jwt.Token, required string) error {
	scopeRaw, ok := tok.Get("scope")
	if !ok {
		return fmt.Errorf("missing required scope: %s", required)
	}
	granted := strings.Fields(fmt.Sprint(scopeRaw))
	for _, s := range granted {
		if s == required {
			return nil
		}
	}
	return fmt.Errorf("missing required scope: %s", required)
}

func claimString(tok jwt.Token, name string) string {
	raw, ok := tok.Get(name)
	if !ok || raw == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(raw))
}

func normalizeIssuer(issuer string) string {
	return strings.TrimSuffix(strings.TrimSpace(issuer), "/")
}
