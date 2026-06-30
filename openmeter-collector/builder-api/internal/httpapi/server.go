package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/livepeer/clearinghouse/openmeter-collector/builder-api/internal/apikey"
	auth0mgmt "github.com/livepeer/clearinghouse/openmeter-collector/builder-api/internal/auth0mgmt"
	"github.com/livepeer/clearinghouse/openmeter-collector/builder-api/internal/auth0mint"
	"github.com/livepeer/clearinghouse/openmeter-collector/builder-api/internal/config"
	"github.com/livepeer/clearinghouse/openmeter-collector/builder-api/internal/openmeter"
)

// Server wires Builder API routes and dependencies.
type Server struct {
	cfg        config.Config
	auth0      *auth0mgmt.Client
	minter     *auth0mint.Minter
	openmeter  *openmeter.Client
	demoKeys   map[string]apikey.DemoEntry
	openAPISpec []byte
}

// NewServer constructs the HTTP API server.
func NewServer(cfg config.Config, auth0 *auth0mgmt.Client, minter *auth0mint.Minter, om *openmeter.Client, demoKeys map[string]apikey.DemoEntry, openAPISpec []byte) *Server {
	return &Server{
		cfg:         cfg,
		auth0:       auth0,
		minter:      minter,
		openmeter:   om,
		demoKeys:    demoKeys,
		openAPISpec: openAPISpec,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /api/v1/openapi.json", s.handleOpenAPI)
	mux.HandleFunc("GET /api/v1/docs", s.handleDocs)
	mux.HandleFunc("POST /api/v1/apps/{clientId}/users", s.handleCreateUser)
	mux.HandleFunc("POST /api/v1/apps/{clientId}/auth/api-key/signer-session", s.handleSignerSession)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(s.openAPISpec)
}

func (s *Server) handleDocs(w http.ResponseWriter, _ *http.Request) {
	html := `<!doctype html>
<html>
  <head>
    <title>Clearinghouse Builder API</title>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
  </head>
  <body>
    <script id="api-reference" data-url="/api/v1/openapi.json" src="https://cdn.jsdelivr.net/npm/@scalar/api-reference@1.61.0"></script>
  </body>
</html>`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

type createUserRequest struct {
	ExternalUserID string `json:"externalUserId"`
	Email          string `json:"email"`
	Connection     string `json:"connection"`
	IssueAPIKey    *bool  `json:"issueApiKey"`
}

type createUserResponse struct {
	ID             string `json:"id"`
	ClientID       string `json:"clientId"`
	ExternalUserID string `json:"externalUserId"`
	Email          string `json:"email,omitempty"`
	Status         string `json:"status"`
	APIKey         string `json:"apiKey,omitempty"`
}

func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	if clientID == "" {
		writeAPIError(w, http.StatusBadRequest, "clientId is required")
		return
	}
	if !M2MAuth(r, s.cfg.SignerM2MClientID, s.cfg.SignerM2MSecret) {
		writeAPIError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	body, err := readJSONBody[createUserRequest](r)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}
	externalUserID := strings.TrimSpace(body.ExternalUserID)
	if externalUserID == "" {
		writeAPIError(w, http.StatusBadRequest, "externalUserId is required")
		return
	}

	issueKey := true
	if body.IssueAPIKey != nil {
		issueKey = *body.IssueAPIKey
	}

	ctx := r.Context()
	user, err := s.auth0.UpsertUser(ctx, clientID, externalUserID, strings.TrimSpace(body.Email), strings.TrimSpace(body.Connection), issueKey, s.cfg.APIKeyPrefix)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := s.openmeter.EnsureCustomer(ctx, clientID, externalUserID, externalUserID); err != nil {
		writeAPIError(w, http.StatusBadGateway, "openmeter customer provisioning failed")
		return
	}

	status := http.StatusOK
	if user.Created {
		status = http.StatusCreated
	}
	writeJSON(w, status, createUserResponse{
		ID:             user.ID,
		ClientID:       clientID,
		ExternalUserID: externalUserID,
		Email:          user.Email,
		Status:         "active",
		APIKey:         user.APIKey,
	})
}

type signerSessionRequest struct {
	Scope string `json:"scope"`
}

type signerSessionResponse struct {
	AccessToken              string `json:"access_token"`
	TokenType                string `json:"token_type"`
	ExpiresIn                int    `json:"expires_in"`
	Scope                    string `json:"scope"`
	BalanceUsdMicros         string `json:"balanceUsdMicros"`
	LifetimeGrantedUsdMicros string `json:"lifetimeGrantedUsdMicros"`
	SignerURL                string `json:"signer_url,omitempty"`
	IssuedTokenType          string `json:"issued_token_type,omitempty"`
	CorrelationID            string `json:"correlation_id,omitempty"`
}

func (s *Server) handleSignerSession(w http.ResponseWriter, r *http.Request) {
	correlationID := newCorrelationID()
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	if clientID == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "clientId is required", correlationID)
		return
	}

	token := BearerToken(r)
	if token == "" {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "missing bearer token", correlationID)
		return
	}
	if apikey.IsM2MSecret(token) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "M2M client secrets cannot be used as API keys", correlationID)
		return
	}
	if !strings.HasPrefix(token, s.cfg.APIKeyPrefix) {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "invalid api key", correlationID)
		return
	}

	var req signerSessionRequest
	if r.ContentLength > 0 {
		parsed, err := readJSONBody[signerSessionRequest](r)
		if err != nil {
			writeOAuthError(w, http.StatusBadRequest, "invalid_request", err.Error(), correlationID)
			return
		}
		req = parsed
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = "sign:job"
	}

	ctx := r.Context()
	resolvedClientID, externalUserID, err := s.resolveAPIKey(ctx, token, clientID)
	if err != nil {
		writeOAuthError(w, http.StatusUnauthorized, "invalid_client", "invalid api key", correlationID)
		return
	}

	if _, err := s.openmeter.EnsureCustomer(ctx, resolvedClientID, externalUserID, externalUserID); err != nil {
		writeOAuthError(w, http.StatusBadGateway, "server_error", "openmeter customer provisioning failed", correlationID)
		return
	}

	minted, err := s.minter.MintSignerToken(ctx, resolvedClientID, externalUserID)
	if err != nil {
		writeOAuthError(w, http.StatusBadGateway, "server_error", "signer token mint failed", correlationID)
		return
	}

	resp := signerSessionResponse{
		AccessToken:              minted.AccessToken,
		TokenType:                "Bearer",
		ExpiresIn:                minted.ExpiresIn,
		Scope:                    scope,
		BalanceUsdMicros:         "0",
		LifetimeGrantedUsdMicros: "0",
		IssuedTokenType:          "urn:ietf:params:oauth:token-type:access_token",
		CorrelationID:            correlationID,
	}
	if s.cfg.SignerURL != "" {
		resp.SignerURL = s.cfg.SignerURL
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) resolveAPIKey(ctx context.Context, token, expectedClientID string) (clientID, externalUserID string, err error) {
	if entry, ok := s.demoKeys[token]; ok {
		if expectedClientID != "" && entry.ClientID != expectedClientID {
			return "", "", errClientMismatch
		}
		return entry.ClientID, entry.UserID, nil
	}
	return s.auth0.ResolveAPIKeyUser(ctx, token, s.cfg.APIKeyPrefix, expectedClientID)
}

var errClientMismatch = errors.New("api key client mismatch")

func readJSONBody[T any](r *http.Request) (T, error) {
	var zero T
	defer r.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return zero, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return zero, nil
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, err
	}
	return out, nil
}
