package tokenexchange_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jws"
	"github.com/lestrrat-go/jwx/v2/jwt"
	auth0mint "github.com/livepeer/clearinghouse/openmeter-collector/builder-api/internal/auth0mint"
	"github.com/livepeer/clearinghouse/openmeter-collector/builder-api/internal/apikey"
	"github.com/livepeer/clearinghouse/openmeter-collector/builder-api/internal/config"
	"github.com/livepeer/clearinghouse/openmeter-collector/builder-api/internal/oidcverify"
	"github.com/livepeer/clearinghouse/openmeter-collector/builder-api/internal/openmeter"
	"github.com/livepeer/clearinghouse/openmeter-collector/builder-api/internal/tokenexchange"
)

type stubMinter struct {
	response *auth0mint.TokenResponse
	err      error
}

func (s stubMinter) MintSignerToken(_ context.Context, _, _ string) (*auth0mint.TokenResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.response, nil
}

type stubOpenMeter struct{}

func (stubOpenMeter) EnsureCustomer(context.Context, string, string, string) (*openmeter.Customer, error) {
	return &openmeter.Customer{}, nil
}

func testHandler(t *testing.T, oidc *oidcverify.Verifier) *tokenexchange.Handler {
	t.Helper()
	cfg := config.Config{
		Auth0Audience:       "livepeer-clearinghouse",
		SignerM2MClientID:   "m2m-client",
		SignerM2MSecret:     "m2m-secret",
		APIKeyPrefix:        "sk_",
		SignerURL:           "http://localhost:8081",
		DiscoveryURL:        "http://localhost/discovery",
	}
	return tokenexchange.NewHandler(
		cfg,
		oidc,
		&apikey.Store{
			Prefix: "sk_",
			Demo: map[string]apikey.DemoEntry{
				"sk_demo": {ClientID: "pub-client", UserID: "demo-user"},
			},
		},
		stubMinter{response: &auth0mint.TokenResponse{
			AccessToken: "minted-jwt",
			TokenType:   "Bearer",
			ExpiresIn:   300,
			Scope:       "sign:job",
		}},
		stubOpenMeter{},
	)
}

func TestExchangeRejectsMissingPublicClientID(t *testing.T) {
	t.Parallel()
	h := testHandler(t, nil)
	_, err := h.Exchange(context.Background(), tokenexchange.Request{
		GrantType:        tokenexchange.GrantType,
		SubjectToken:     "sk_demo",
		SubjectTokenType: tokenexchange.SubjectAccessTokenType,
	}, "corr")
	if err == nil || err.(*tokenexchange.Error).Code != "invalid_request" {
		t.Fatalf("expected invalid_request, got %v", err)
	}
}

func TestExchangeRejectsInvalidGrantType(t *testing.T) {
	t.Parallel()
	h := testHandler(t, nil)
	_, err := h.Exchange(context.Background(), tokenexchange.Request{
		PublicClientID:   "pub-client",
		ClientID:         "m2m-client",
		ClientSecret:     "m2m-secret",
		GrantType:        "client_credentials",
		SubjectToken:     "token",
		SubjectTokenType: tokenexchange.SubjectAccessTokenType,
	}, "corr")
	if err == nil || err.(*tokenexchange.Error).Code != "invalid_request" {
		t.Fatalf("expected invalid_request, got %v", err)
	}
}

func TestExchangeRejectsInvalidClient(t *testing.T) {
	t.Parallel()
	h := testHandler(t, nil)
	_, err := h.Exchange(context.Background(), tokenexchange.Request{
		ClientID:         "wrong",
		ClientSecret:     "secret",
		GrantType:        tokenexchange.GrantType,
		SubjectToken:     "sk_demo",
		SubjectTokenType: tokenexchange.SubjectAccessTokenType,
	}, "corr")
	if err == nil || err.(*tokenexchange.Error).Code != "invalid_client" {
		t.Fatalf("expected invalid_client, got %v", err)
	}
}

func TestExchangeRejectsUnsupportedSubjectTokenType(t *testing.T) {
	t.Parallel()
	h := testHandler(t, nil)
	_, err := h.Exchange(context.Background(), tokenexchange.Request{
		PublicClientID:   "pub-client",
		ClientID:         "m2m-client",
		ClientSecret:     "m2m-secret",
		GrantType:        tokenexchange.GrantType,
		SubjectToken:     "token",
		SubjectTokenType: "urn:ietf:params:oauth:token-type:id_token",
	}, "corr")
	if err == nil || err.(*tokenexchange.Error).Code != "unsupported_token_type" {
		t.Fatalf("expected unsupported_token_type, got %v", err)
	}
}

func TestExchangeRejectsInvalidTargetAudience(t *testing.T) {
	t.Parallel()
	h := testHandler(t, nil)
	_, err := h.Exchange(context.Background(), tokenexchange.Request{
		PublicClientID:   "pub-client",
		ClientID:         "m2m-client",
		ClientSecret:     "m2m-secret",
		GrantType:        tokenexchange.GrantType,
		SubjectToken:     "sk_demo",
		SubjectTokenType: tokenexchange.SubjectAccessTokenType,
		Audiences:        []string{"other-audience"},
	}, "corr")
	if err == nil || err.(*tokenexchange.Error).Code != "invalid_target" {
		t.Fatalf("expected invalid_target, got %v", err)
	}
}

func TestExchangeAPIKeyHappyPathWithoutM2M(t *testing.T) {
	t.Parallel()
	h := testHandler(t, nil)
	result, err := h.Exchange(context.Background(), tokenexchange.Request{
		PublicClientID:   "pub-client",
		GrantType:        tokenexchange.GrantType,
		SubjectToken:     "sk_demo",
		SubjectTokenType: tokenexchange.SubjectAccessTokenType,
		Audiences:        []string{"livepeer-clearinghouse"},
	}, "corr-0")
	if err != nil {
		t.Fatal(err)
	}
	if result.AccessToken != "minted-jwt" {
		t.Fatalf("access_token = %q", result.AccessToken)
	}
}

func TestExchangeAPIKeyHappyPath(t *testing.T) {
	t.Parallel()
	h := testHandler(t, nil)
	result, err := h.Exchange(context.Background(), tokenexchange.Request{
		PublicClientID:   "pub-client",
		ClientID:         "m2m-client",
		ClientSecret:     "m2m-secret",
		GrantType:        tokenexchange.GrantType,
		SubjectToken:     "sk_demo",
		SubjectTokenType: tokenexchange.SubjectAccessTokenType,
		Audiences:        []string{"livepeer-clearinghouse"},
	}, "corr-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.AccessToken != "minted-jwt" {
		t.Fatalf("access_token = %q", result.AccessToken)
	}
	if result.Scope != "sign:job" {
		t.Fatalf("scope = %q", result.Scope)
	}
	if result.IssuedTokenType != tokenexchange.IssuedAccessTokenType {
		t.Fatalf("issued_token_type = %q", result.IssuedTokenType)
	}
}

func TestExchangeJWTHappyPath(t *testing.T) {
	t.Parallel()
	privateKey, jwksServer := testJWKS(t)
	t.Cleanup(jwksServer.Close)

	verifier, err := oidcverify.NewWithJWKSURL(
		context.Background(),
		"https://idp.test",
		"livepeer-clearinghouse",
		jwksServer.URL,
		oidcverify.Options{RequiredScopes: []string{"sign:job"}},
	)
	if err != nil {
		t.Fatal(err)
	}

	token := signJWT(t, privateKey, "pub-client", "user-1")
	h := testHandler(t, verifier)
	result, err := h.Exchange(context.Background(), tokenexchange.Request{
		PublicClientID:   "pub-client",
		ClientID:         "m2m-client",
		ClientSecret:     "m2m-secret",
		GrantType:        tokenexchange.GrantType,
		SubjectToken:     token,
		SubjectTokenType: tokenexchange.SubjectAccessTokenType,
		Resource:         "livepeer-clearinghouse",
	}, "corr-2")
	if err != nil {
		t.Fatal(err)
	}
	if result.AccessToken != "minted-jwt" {
		t.Fatalf("access_token = %q", result.AccessToken)
	}
}

func testJWKS(t *testing.T) (*rsa.PrivateKey, *httptest.Server) {
	t.Helper()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	publicJWK, err := jwk.FromRaw(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	_ = publicJWK.Set(jwk.KeyIDKey, "k")
	_ = publicJWK.Set(jwk.AlgorithmKey, jwa.RS256)
	keySet := jwk.NewSet()
	_ = keySet.AddKey(publicJWK)
	jwksBody, err := json.Marshal(keySet)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksBody)
	}))
	return privateKey, server
}

func signJWT(t *testing.T, privateKey *rsa.PrivateKey, clientID, subject string) string {
	t.Helper()
	tok, err := jwt.NewBuilder().
		Issuer("https://idp.test/").
		Audience([]string{"livepeer-clearinghouse"}).
		Subject(subject).
		Claim("azp", clientID).
		Claim("scope", "sign:job").
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(5 * time.Minute)).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	headers := jws.NewHeaders()
	_ = headers.Set(jws.KeyIDKey, "k")
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, privateKey, jws.WithProtectedHeaders(headers)))
	if err != nil {
		t.Fatal(err)
	}
	return string(signed)
}
