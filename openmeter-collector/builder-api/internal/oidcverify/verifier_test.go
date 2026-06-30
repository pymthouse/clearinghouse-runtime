package oidcverify

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
)

func TestVerifyUserAccessToken(t *testing.T) {
	t.Parallel()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	publicJWK, err := jwk.FromRaw(&privateKey.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	_ = publicJWK.Set(jwk.KeyIDKey, "test-key")
	_ = publicJWK.Set(jwk.AlgorithmKey, jwa.RS256)
	keySet := jwk.NewSet()
	_ = keySet.AddKey(publicJWK)

	jwksBody, err := json.Marshal(keySet)
	if err != nil {
		t.Fatal(err)
	}

	issuer := "https://idp.test"
	audience := "livepeer-clearinghouse"
	clientID := "pub-client"
	subject := "google-oauth2|105691875604954324733"

	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksBody)
	}))
	t.Cleanup(jwksServer.Close)

	// Point issuer at test server path for JWKS only.
	verifier := &Verifier{
		issuer:   issuer,
		audience: audience,
		cache:    jwk.NewCache(context.Background()),
		jwksURL:  jwksServer.URL,
	}
	if err := verifier.cache.Register(jwksServer.URL, jwk.WithMinRefreshInterval(time.Hour)); err != nil {
		t.Fatal(err)
	}

	token := buildTestToken(t, privateKey, issuer, audience, clientID, subject, "sign:job openid")

	user, err := verifier.VerifyUserAccessToken(context.Background(), token, clientID)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if user.ClientID != clientID {
		t.Fatalf("clientID = %q", user.ClientID)
	}
	if user.ExternalUserID != subject {
		t.Fatalf("externalUserID = %q", user.ExternalUserID)
	}

	if _, err := verifier.VerifyUserAccessToken(context.Background(), token, "other-client"); err == nil {
		t.Fatal("expected azp mismatch error")
	}

	badScope := buildTestToken(t, privateKey, issuer, audience, clientID, subject, "openid")
	if _, err := verifier.VerifyUserAccessToken(context.Background(), badScope, clientID); err == nil {
		t.Fatal("expected missing scope error")
	}
}

func buildTestToken(
	t *testing.T,
	privateKey *rsa.PrivateKey,
	issuer, audience, clientID, subject, scope string,
) string {
	t.Helper()
	tok, err := jwt.NewBuilder().
		Issuer(issuer + "/").
		Audience([]string{audience}).
		Subject(subject).
		Claim("azp", clientID).
		Claim("scope", scope).
		IssuedAt(time.Now()).
		Expiration(time.Now().Add(5 * time.Minute)).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	headers := jws.NewHeaders()
	_ = headers.Set(jws.KeyIDKey, "test-key")
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, privateKey, jws.WithProtectedHeaders(headers)))
	if err != nil {
		t.Fatal(err)
	}
	return string(signed)
}
