package oidcverify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

// Options configures claim extraction and scope requirements for incoming user JWTs.
type Options struct {
	ClientClaim    string
	SubjectClaim   string
	RequiredScopes []string
}

func (o Options) withDefaults() Options {
	if strings.TrimSpace(o.ClientClaim) == "" {
		o.ClientClaim = "azp"
	}
	if strings.TrimSpace(o.SubjectClaim) == "" {
		o.SubjectClaim = "sub"
	}
	if len(o.RequiredScopes) == 0 {
		o.RequiredScopes = []string{"sign:job"}
	}
	return o
}

// VerifiedUser is an end-user identity extracted from an Auth0 access token.
type VerifiedUser struct {
	ClientID       string
	ExternalUserID string
}

// Verifier validates Auth0 end-user access tokens (device code, authorization code).
type Verifier struct {
	issuer   string
	audience string
	opts     Options
	cache    *jwk.Cache
	jwksURL  string
}

// New creates a JWKS-cached OIDC access-token verifier.
func New(ctx context.Context, issuer, audience string, opts Options) (*Verifier, error) {
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
		opts:     opts.withDefaults(),
		cache:    cache,
		jwksURL:  jwksURL,
	}, nil
}

// NewWithJWKSURL creates a verifier that loads keys from an explicit JWKS URL (tests or OIDC_JWKS_URI overrides).
func NewWithJWKSURL(ctx context.Context, issuer, audience, jwksURL string, opts Options) (*Verifier, error) {
	issuer = strings.TrimSuffix(strings.TrimSpace(issuer), "/")
	audience = strings.TrimSpace(audience)
	jwksURL = strings.TrimSpace(jwksURL)
	if issuer == "" {
		return nil, fmt.Errorf("oidcverify: issuer is required")
	}
	if audience == "" {
		return nil, fmt.Errorf("oidcverify: audience is required")
	}
	if jwksURL == "" {
		return nil, fmt.Errorf("oidcverify: jwksURL is required")
	}

	cache := jwk.NewCache(ctx)
	if err := cache.Register(jwksURL, jwk.WithMinRefreshInterval(15*time.Minute)); err != nil {
		return nil, fmt.Errorf("oidcverify: register jwks: %w", err)
	}

	return &Verifier{
		issuer:   issuer,
		audience: audience,
		opts:     opts.withDefaults(),
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

	for _, scope := range v.opts.RequiredScopes {
		if err := requireScope(parsed, scope); err != nil {
			return nil, err
		}
	}

	clientID := claimString(parsed, v.opts.ClientClaim)
	if clientID == "" {
		clientID = claimString(parsed, "azp")
	}
	if clientID == "" {
		return nil, fmt.Errorf("token missing %s claim", v.opts.ClientClaim)
	}
	expectedClientID = strings.TrimSpace(expectedClientID)
	if expectedClientID != "" && clientID != expectedClientID {
		return nil, fmt.Errorf("token azp does not match clientId")
	}

	externalUserID := claimString(parsed, v.opts.SubjectClaim)
	if externalUserID == "" {
		externalUserID = claimString(parsed, "external_user_id")
	}
	if externalUserID == "" {
		externalUserID = parsed.Subject()
	}
	if externalUserID == "" {
		return nil, fmt.Errorf("token missing %s claim", v.opts.SubjectClaim)
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
