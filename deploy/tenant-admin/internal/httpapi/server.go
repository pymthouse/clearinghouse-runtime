package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/livepeer/clearinghouse/deploy/tenant-admin/internal/tenant"
)

type Server struct {
	adminSecret string
	provisioner *tenant.Provisioner
}

func NewServer(adminSecret string, provisioner *tenant.Provisioner) *Server {
	return &Server{
		adminSecret: strings.TrimSpace(adminSecret),
		provisioner: provisioner,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/admin/tenants", s.handleTenants)
	mux.HandleFunc("/admin/tenants/", s.handleTenantByID)
	mux.HandleFunc("/admin/customers", s.handleEnsureCustomer)
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleTenants(w http.ResponseWriter, r *http.Request) {
	if !s.isAuthorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	switch r.Method {
	case http.MethodPost:
		s.handleProvisionTenant(w, r)
	case http.MethodGet:
		records, err := s.provisioner.ListTenants(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"tenants": records,
		})
	default:
		http.NotFound(w, r)
	}
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
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request json"})
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
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleTenantByID(w http.ResponseWriter, r *http.Request) {
	if !s.isAuthorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/admin/tenants/")
	path = strings.TrimSpace(path)
	if path == "" {
		http.NotFound(w, r)
		return
	}
	pathParts := strings.Split(path, "/")
	tenantID := strings.TrimSpace(pathParts[0])
	if tenantID == "" {
		http.NotFound(w, r)
		return
	}
	if len(pathParts) == 1 && r.Method == http.MethodGet {
		tenantRecord, appRecords, err := s.provisioner.GetTenant(r.Context(), tenantID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if tenantRecord == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "tenant not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"tenant": tenantRecord,
			"apps":   appRecords,
		})
		return
	}
	if len(pathParts) == 2 && pathParts[1] == "usage" && r.Method == http.MethodGet {
		s.handleTenantUsage(w, r, tenantID)
		return
	}
	if len(pathParts) == 3 && pathParts[1] == "usage" && pathParts[2] == "balance" && r.Method == http.MethodGet {
		s.handleTenantUsageBalance(w, r, tenantID)
		return
	}
	if len(pathParts) == 3 && pathParts[1] == "spat" && pathParts[2] == "rotate" && r.Method == http.MethodPost {
		s.handleRotateTenantSPAT(w, r, tenantID)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) handleTenantUsage(w http.ResponseWriter, r *http.Request, tenantID string) {
	clientID := strings.TrimSpace(r.URL.Query().Get("clientId"))
	externalUserID := strings.TrimSpace(r.URL.Query().Get("externalUserId"))
	startDate := strings.TrimSpace(r.URL.Query().Get("startDate"))
	endDate := strings.TrimSpace(r.URL.Query().Get("endDate"))
	if clientID == "" {
		_, appRecords, err := s.provisioner.GetTenant(r.Context(), tenantID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		summaries := make([]any, 0, len(appRecords))
		for _, appRecord := range appRecords {
			summary, readErr := s.provisioner.ReadUsage(
				r.Context(),
				tenantID,
				appRecord.ClientID,
				externalUserID,
				startDate,
				endDate,
			)
			if readErr != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": readErr.Error()})
				return
			}
			summaries = append(summaries, summary)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"tenantId": tenantID,
			"usage":    summaries,
		})
		return
	}
	result, err := s.provisioner.ReadUsage(r.Context(), tenantID, clientID, externalUserID, startDate, endDate)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleTenantUsageBalance(w http.ResponseWriter, r *http.Request, tenantID string) {
	clientID := strings.TrimSpace(r.URL.Query().Get("clientId"))
	externalUserID := strings.TrimSpace(r.URL.Query().Get("externalUserId"))
	featureKey := strings.TrimSpace(r.URL.Query().Get("featureKey"))
	if clientID == "" || externalUserID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "clientId and externalUserId are required"})
		return
	}
	result, err := s.provisioner.ReadBalance(r.Context(), tenantID, clientID, externalUserID, featureKey)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) handleRotateTenantSPAT(w http.ResponseWriter, r *http.Request, tenantID string) {
	token, envFilePath, err := s.provisioner.RotateSPAT(r.Context(), tenantID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"tenantId": tenantID,
		"tokenId":  token.TokenID,
		"spat":     token.Token,
		"envFile":  envFilePath,
	})
}

func (s *Server) handleEnsureCustomer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if !s.isAuthorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	var payload struct {
		TenantID       string `json:"tenantId"`
		ClientID       string `json:"clientId"`
		ExternalUserID string `json:"externalUserId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request json"})
		return
	}
	result, err := s.provisioner.EnsureCustomer(
		context.Background(),
		payload.TenantID,
		payload.ClientID,
		payload.ExternalUserID,
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) isAuthorized(r *http.Request) bool {
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	return token != "" && token == s.adminSecret
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
