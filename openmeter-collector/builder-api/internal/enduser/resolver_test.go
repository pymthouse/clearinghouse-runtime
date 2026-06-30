package enduser

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
	"github.com/livepeer/clearinghouse/openmeter-collector/builder-api/internal/apikey"
	"github.com/livepeer/clearinghouse/openmeter-collector/builder-api/internal/oidcverify"
)

func TestResolverAPIKeyWhenNotJWT(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{
		APIKeys: &apikey.Store{
			Prefix: "sk_",
			Demo: map[string]apikey.DemoEntry{
				"sk_demo": {ClientID: "demo-client", UserID: "demo-user"},
			},
		},
		Prefix: "sk_",
	}

	identity, err := resolver.ResolveBearer(context.Background(), "sk_demo", "demo-client")
	if err != nil {
		t.Fatal(err)
	}
	if identity.ExternalUserID != "demo-user" {
		t.Fatalf("usage_subject = %q", identity.ExternalUserID)
	}
}

func TestResolverJWTWinsOverAPIKeyStore(t *testing.T) {
	t.Parallel()

	privateKey, jwksServer := testJWKS(t)
	t.Cleanup(jwksServer.Close)

	verifier, err := oidcverify.NewWithJWKSURL(
		context.Background(),
		"https://idp.test",
		"clearinghouse",
		jwksServer.URL,
		oidcverify.Options{RequiredScopes: []string{"sign:job"}},
	)
	if err != nil {
		t.Fatal(err)
	}

	clientID := "app-b"
	subject := "user-b"
	token := signTestJWT(t, privateKey, "https://idp.test", "clearinghouse", map[string]any{
		"azp":   clientID,
		"scope": "sign:job",
	}, subject)

	resolver := &Resolver{
		OIDC: verifier,
		APIKeys: &apikey.Store{
			Prefix: "sk_",
			Demo: map[string]apikey.DemoEntry{
				"sk_demo": {ClientID: "demo-client", UserID: "demo-user"},
			},
		},
		Prefix: "sk_",
	}

	identity, err := resolver.ResolveBearer(context.Background(), token, clientID)
	if err != nil {
		t.Fatal(err)
	}
	if identity.ExternalUserID != subject {
		t.Fatalf("usage_subject = %q", identity.ExternalUserID)
	}
}

func TestResolverRejectsM2MSecret(t *testing.T) {
	t.Parallel()

	resolver := &Resolver{Prefix: "sk_"}
	_, err := resolver.ResolveBearer(context.Background(), "pmth_cs_secret", "demo-client")
	if err == nil {
		t.Fatal("expected error")
	}
	if OAuthErrorCode(err) != "invalid_request" {
		t.Fatalf("code = %q", OAuthErrorCode(err))
	}
}

func TestResolverJWTDoesNotFallThroughToAPIKey(t *testing.T) {
	t.Parallel()

	_, jwksServer := testJWKS(t)
	t.Cleanup(jwksServer.Close)

	verifier, err := oidcverify.NewWithJWKSURL(
		context.Background(),
		"https://idp.test",
		"clearinghouse",
		jwksServer.URL,
		oidcverify.Options{RequiredScopes: []string{"sign:job"}},
	)
	if err != nil {
		t.Fatal(err)
	}

	resolver := &Resolver{
		OIDC: verifier,
		APIKeys: &apikey.Store{
			Prefix: "sk_",
			Demo: map[string]apikey.DemoEntry{
				"sk_notused": {ClientID: "demo-client", UserID: "demo-user"},
			},
		},
		Prefix: "sk_",
	}

	_, err = resolver.ResolveBearer(context.Background(), "a.b.c", "demo-client")
	if err == nil {
		t.Fatal("expected invalid token")
	}
	if OAuthErrorCode(err) != "invalid_token" {
		t.Fatalf("code = %q", OAuthErrorCode(err))
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

func signTestJWT(
	t *testing.T,
	privateKey *rsa.PrivateKey,
	issuer, audience string,
	claims map[string]any,
	subject string,
) string {
	t.Helper()
	builder := jwt.NewBuilder().
		Issuer(issuer + "/").
		Audience([]string{audience}).
		Subject(subject).
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(5 * time.Minute))
	for k, v := range claims {
		builder = builder.Claim(k, v)
	}
	tok, err := builder.Build()
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
