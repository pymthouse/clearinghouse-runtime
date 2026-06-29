package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/livepeer/clearinghouse/tenant-admin/internal/customerkey"
	"github.com/livepeer/clearinghouse/tenant-admin/internal/tenant"
)

type Server struct {
	adminSecret    string
	internalSecret string
	provisioner    *tenant.Provisioner
}

func NewServer(adminSecret, internalSecret string, provisioner *tenant.Provisioner) *Server {
	return &Server{
		adminSecret:    strings.TrimSpace(adminSecret),
		internalSecret: strings.TrimSpace(internalSecret),
		provisioner:    provisioner,
	}
}

// AdminHandler serves the admin-only control-plane API. Every route except /health
// requires the ADMIN_SECRET bearer token.
func (s *Server) AdminHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /v1/tenants", s.requireAdmin(s.handleProvisionTenant))
	mux.HandleFunc("GET /v1/tenants", s.requireAdmin(s.handleListTenants))
	mux.HandleFunc("GET /v1/tenants/{tenantId}", s.requireAdmin(s.handleGetTenant))
	mux.HandleFunc("PUT /v1/tenants/{tenantId}/apps/{clientId}", s.requireAdmin(s.handleUpsertApp))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/tokens", s.requireAdmin(s.handleCreateToken))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/tokens/{tokenId}/rotate", s.requireAdmin(s.handleRotateToken))
	return mux
}

// InternalHandler serves loopback/private-network-only endpoints. Bind it to a
// loopback address (see config.InternalListenAddr); it carries no auth of its own.
func (s *Server) InternalHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /internal/customers/ensure", s.requireInternalSecret(s.handleEnsureCustomer))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleProvisionTenant(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		TenantID         string   `json:"tenantId"`
		TenantName       string   `json:"tenantName"`
		AdminEmails      []string `json:"adminEmails"`
		AdminPassword    string   `json:"adminPassword"`
		ClientID         string   `json:"clientId"`
		ExternalUserID   string   `json:"externalUserId"`
		EnableSampleUser bool     `json:"enableSampleUser"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request json")
		return
	}
	result, err := s.provisioner.ProvisionTenant(r.Context(), tenant.TenantProvisionInput{
		TenantID:         payload.TenantID,
		TenantName:       payload.TenantName,
		AdminEmails:      payload.AdminEmails,
		AdminPassword:    payload.AdminPassword,
		ClientID:         payload.ClientID,
		ExternalUserID:   payload.ExternalUserID,
		EnableSampleUser: payload.EnableSampleUser,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleListTenants(w http.ResponseWriter, r *http.Request) {
	records, err := s.provisioner.ListTenants(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenants": records})
}

func (s *Server) handleGetTenant(w http.ResponseWriter, r *http.Request) {
	tenantID := strings.TrimSpace(r.PathValue("tenantId"))
	tenantRecord, appRecords, err := s.provisioner.GetTenant(r.Context(), tenantID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if tenantRecord == nil {
		writeError(w, http.StatusNotFound, "tenant not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenant": tenantRecord, "apps": appRecords})
}

func (s *Server) handleUpsertApp(w http.ResponseWriter, r *http.Request) {
	tenantID := strings.TrimSpace(r.PathValue("tenantId"))
	clientID := strings.TrimSpace(r.PathValue("clientId"))
	if clientID == "" {
		writeError(w, http.StatusBadRequest, "clientId is required")
		return
	}
	if err := s.provisioner.RegisterApp(r.Context(), tenantID, clientID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tenantId": tenantID, "clientId": clientID})
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	tenantID := strings.TrimSpace(r.PathValue("tenantId"))
	var payload struct {
		Kind string `json:"kind"`
	}
	// Body is optional; default to ingest.
	_ = json.NewDecoder(r.Body).Decode(&payload)
	switch strings.ToLower(strings.TrimSpace(payload.Kind)) {
	case "", "ingest":
		token, err := s.provisioner.CreateIngestToken(r.Context(), tenantID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"tenantId": tenantID,
			"kind":     "ingest",
			"tokenId":  token.TokenID,
			"spat":     token.Token,
		})
	case "reader":
		// SECURITY GATE: reader tokens grant direct OpenMeter read access. They are not
		// issued until we confirm Konnect can scope Metering/Billing roles to a per-tenant
		// resource boundary; a wildcard read scope would leak cross-tenant billing data.
		writeError(w, http.StatusNotImplemented,
			"reader tokens are gated: per-tenant Metering/Billing role scoping is unconfirmed")
	default:
		writeError(w, http.StatusBadRequest, "kind must be \"ingest\" or \"reader\"")
	}
}

func (s *Server) handleRotateToken(w http.ResponseWriter, r *http.Request) {
	tenantID := strings.TrimSpace(r.PathValue("tenantId"))
	tokenID := strings.TrimSpace(r.PathValue("tokenId"))
	token, err := s.provisioner.RotateIngestToken(r.Context(), tenantID, tokenID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenantId": tenantID,
		"kind":     "ingest",
		"tokenId":  token.TokenID,
		"spat":     token.Token,
	})
}

// handleEnsureCustomer ensures the OpenMeter customer for an (app, end-user) identity.
// Identity is the builder-sdk shape: clientId + externalUserId, or a single subject /
// authId ("clientId:externalUserId") that is split on the first colon.
func (s *Server) handleEnsureCustomer(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ClientID       string `json:"clientId"`
		ExternalUserID string `json:"externalUserId"`
		Subject        string `json:"subject"`
		AuthID         string `json:"authId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request json")
		return
	}
	clientID := strings.TrimSpace(payload.ClientID)
	externalUserID := strings.TrimSpace(payload.ExternalUserID)
	if clientID == "" || externalUserID == "" {
		subject := strings.TrimSpace(payload.Subject)
		if subject == "" {
			subject = strings.TrimSpace(payload.AuthID)
		}
		if c, e, ok := customerkey.Parse(subject); ok {
			clientID, externalUserID = c, e
		}
	}
	if clientID == "" || externalUserID == "" {
		writeError(w, http.StatusBadRequest, "clientId and externalUserId (or subject) are required")
		return
	}
	result, err := s.provisioner.EnsureCustomer(r.Context(), clientID, externalUserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthorized(r) {
			writeError(w, http.StatusUnauthorized, "unauthorized")
			return
		}
		next(w, r)
	}
}

// requireInternalSecret gates the internal API. The internal listener is expected to be
// bound to loopback or a private network; if INTERNAL_API_SECRET is also configured, an
// X-Internal-Secret header is required as defense in depth.
func (s *Server) requireInternalSecret(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.internalSecret != "" {
			provided := strings.TrimSpace(r.Header.Get("X-Internal-Secret"))
			if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(s.internalSecret)) != 1 {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) isAuthorized(r *http.Request) bool {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" || s.adminSecret == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(s.adminSecret)) == 1
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
