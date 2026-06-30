package enduser

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/livepeer/clearinghouse/openmeter-collector/builder-api/internal/apikey"
	"github.com/livepeer/clearinghouse/openmeter-collector/builder-api/internal/oidcverify"
)

// Identity is the resolved end-user tenant + subject for signer-session exchange.
type Identity struct {
	ClientID       string
	ExternalUserID string
}

// ResolveError carries an OAuth error code for HTTP mapping.
type ResolveError struct {
	Code    string
	Message string
	Err     error
}

func (e *ResolveError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "resolve failed"
}

// Resolver resolves bearer credentials via OIDC (JWT first) then API key.
type Resolver struct {
	OIDC    *oidcverify.Verifier
	APIKeys *apikey.Store
	Prefix  string
}

// ResolveBearer accepts a subject token for RFC 8693 exchange: JWT-shaped tokens
// are verified via OIDC; otherwise sk_* API keys are resolved from the key store.
func (r *Resolver) ResolveBearer(ctx context.Context, bearer, pathClientID string) (Identity, error) {
	bearer = strings.TrimSpace(bearer)
	if bearer == "" {
		return Identity{}, &ResolveError{
			Code:    "invalid_client",
			Message: "missing bearer token",
			Err:     errors.New("missing bearer token"),
		}
	}
	if apikey.IsM2MSecret(bearer) {
		return Identity{}, &ResolveError{
			Code:    "invalid_request",
			Message: "M2M client secrets cannot be used as API keys",
			Err:     errors.New("m2m secret"),
		}
	}

	if strings.Count(bearer, ".") == 2 {
		if r.OIDC == nil {
			return Identity{}, &ResolveError{
				Code:    "invalid_token",
				Message: "oidc verification not configured",
				Err:     errors.New("oidc verification not configured"),
			}
		}
		verified, err := r.OIDC.VerifyUserAccessToken(ctx, bearer, pathClientID)
		if err != nil {
			return Identity{}, &ResolveError{
				Code:    "invalid_token",
				Message: err.Error(),
				Err:     err,
			}
		}
		return Identity{
			ClientID:       verified.ClientID,
			ExternalUserID: verified.ExternalUserID,
		}, nil
	}

	if !strings.HasPrefix(bearer, r.Prefix) {
		return Identity{}, &ResolveError{
			Code:    "invalid_client",
			Message: "invalid api key",
			Err:     errors.New("invalid api key"),
		}
	}
	if r.APIKeys == nil {
		return Identity{}, &ResolveError{
			Code:    "invalid_client",
			Message: "invalid api key",
			Err:     errors.New("invalid api key"),
		}
	}

	clientID, externalUserID, err := r.APIKeys.Resolve(ctx, bearer, pathClientID)
	if err != nil {
		msg := "invalid api key"
		if errors.Is(err, apikey.ErrClientMismatch) {
			msg = "api key client mismatch"
		}
		return Identity{}, &ResolveError{
			Code:    "invalid_client",
			Message: msg,
			Err:     err,
		}
	}
	return Identity{
		ClientID:       clientID,
		ExternalUserID: externalUserID,
	}, nil
}

// OAuthHTTPStatus maps a ResolveError to an HTTP status code.
func OAuthHTTPStatus(err error) int {
	var re *ResolveError
	if errors.As(err, &re) {
		switch re.Code {
		case "invalid_request":
			return 400
		case "invalid_token":
			return 401
		default:
			return 401
		}
	}
	return 401
}

// OAuthErrorCode returns the OAuth error code from a ResolveError.
func OAuthErrorCode(err error) string {
	var re *ResolveError
	if errors.As(err, &re) && re.Code != "" {
		return re.Code
	}
	return "invalid_client"
}

// OAuthErrorDescription returns a client-facing error description.
func OAuthErrorDescription(err error) string {
	var re *ResolveError
	if errors.As(err, &re) && re.Message != "" {
		return re.Message
	}
	return fmt.Sprintf("%v", err)
}
