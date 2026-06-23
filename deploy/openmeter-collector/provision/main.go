package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
)

type ensureRequest struct {
	TenantID        string         `json:"tenant_id"`
	TenantIDAlt     string         `json:"tenantId"`
	ClientID        string         `json:"client_id"`
	ClientIDAlt     string         `json:"clientId"`
	ExternalUserID  string         `json:"external_user_id"`
	ExternalUserAlt string         `json:"externalUserId"`
	AuthID          string         `json:"auth_id"`
	AuthIDAlt       string         `json:"authId"`
	Subject         string         `json:"subject"`
	Data            map[string]any `json:"data"`
}

func requiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func stringField(body map[string]any, keys ...string) string {
	for _, key := range keys {
		if raw, ok := body[key]; ok {
			if value, ok := raw.(string); ok {
				if trimmed := strings.TrimSpace(value); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func parseIdentity(body ensureRequest, defaultTenantID string) (tenantID, clientID, externalUserID string, ok bool) {
	data := body.Data
	if data == nil {
		data = map[string]any{}
	}

	tenantID = firstNonEmpty(
		body.TenantID,
		body.TenantIDAlt,
		stringField(data, "tenant_id", "tenantId"),
	)
	clientID = firstNonEmpty(
		body.ClientID,
		body.ClientIDAlt,
		stringField(data, "client_id", "clientId"),
	)
	externalUserID = firstNonEmpty(
		body.ExternalUserID,
		body.ExternalUserAlt,
		stringField(data, "external_user_id", "externalUserId"),
	)
	if tenantID != "" && clientID != "" && externalUserID != "" {
		return tenantID, clientID, externalUserID, true
	}

	authID := firstNonEmpty(
		body.AuthID,
		body.AuthIDAlt,
		stringField(data, "auth_id", "authId"),
		body.Subject,
	)
	if parsedTenantID, parsedClientID, parsedExternalUserID, parsed := parseAuthID(authID); parsed {
		return parsedTenantID, parsedClientID, parsedExternalUserID, true
	}
	if legacyClientID, legacyExternalUserID, parsed := parseLegacyAuthID(authID); parsed {
		fallbackTenantID := strings.TrimSpace(defaultTenantID)
		if fallbackTenantID != "" {
			return fallbackTenantID, legacyClientID, legacyExternalUserID, true
		}
	}
	return "", "", "", false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func rewriteIngestPayload(payload map[string]any) ([]byte, error) {
	data, _ := payload["data"].(map[string]any)
	tenantID := firstNonEmpty(
		stringField(payload, "tenant_id", "tenantId"),
		stringField(data, "tenant_id", "tenantId"),
	)
	clientID := stringField(data, "client_id", "clientId")
	externalUserID := stringField(data, "external_user_id", "externalUserId")
	if tenantID == "" || clientID == "" || externalUserID == "" {
		subject := firstNonEmpty(stringField(payload, "subject"), stringField(data, "auth_id", "authId"))
		if parsedTenantID, parsedClientID, parsedExternalUserID, parsed := parseAuthID(subject); parsed {
			tenantID, clientID, externalUserID = parsedTenantID, parsedClientID, parsedExternalUserID
		}
	}
	if tenantID == "" || clientID == "" || externalUserID == "" {
		return nil, fmt.Errorf("tenant_id, client_id and external_user_id are required for ingest rewrite")
	}
	customerKey, err := buildCustomerKey(tenantID, clientID, externalUserID)
	if err != nil {
		return nil, err
	}
	payload["subject"] = customerKey
	return json.Marshal(payload)
}

func main() {
	port := strings.TrimSpace(os.Getenv("PROVISION_PORT"))
	if port == "" {
		port = "8091"
	}
	tenantDataDir := strings.TrimSpace(os.Getenv("TENANT_ADMIN_DATA_DIR"))
	if tenantDataDir == "" {
		tenantDataDir = "/tenant-admin-data"
	}
	planKey := strings.TrimSpace(os.Getenv("OPENMETER_DEFAULT_PLAN_KEY"))
	if planKey == "" {
		planKey = "clearinghouse_default_ppu"
	}

	baseURL, err := requiredEnv("OPENMETER_URL")
	if err != nil {
		log.Fatal(err)
	}
	apiKey, err := requiredEnv("OPENMETER_API_KEY")
	if err != nil {
		log.Fatal(err)
	}
	ingestURL := strings.TrimSpace(os.Getenv("OPENMETER_INGEST_URL"))
	if ingestURL == "" {
		ingestURL = strings.TrimRight(normalizeOpenMeterURL(baseURL), "/") + "/v3/openmeter/events"
	}

	defaultTenantID := strings.TrimSpace(os.Getenv("DEFAULT_TENANT_ID"))
	provisioners := map[string]*Provisioner{}
	var provisionersMu sync.Mutex

	getProvisionerForTenant := func(tenantID string) (*Provisioner, string, error) {
		trimmedTenantID := strings.TrimSpace(tenantID)
		if trimmedTenantID == "" {
			return nil, "", fmt.Errorf("tenant_id is required")
		}
		provisionersMu.Lock()
		defer provisionersMu.Unlock()
		if existingProvisioner, ok := provisioners[trimmedTenantID]; ok {
			token, tokenErr := loadTenantOpenMeterAPIKey(tenantDataDir, trimmedTenantID)
			if tokenErr == nil && strings.TrimSpace(token) != "" {
				return existingProvisioner, strings.TrimSpace(token), nil
			}
			return existingProvisioner, apiKey, nil
		}
		token, tokenErr := loadTenantOpenMeterAPIKey(tenantDataDir, trimmedTenantID)
		if tokenErr != nil || strings.TrimSpace(token) == "" {
			token = apiKey
		}
		provisioner := NewProvisioner(baseURL, token, planKey)
		provisioners[trimmedTenantID] = provisioner
		return provisioner, token, nil
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/ensure", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		var body ensureRequest
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request json"})
				return
			}
		}

		tenantID, clientID, externalUserID, ok := parseIdentity(body, defaultTenantID)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "tenant_id, client_id and external_user_id are required (legacy auth_id requires DEFAULT_TENANT_ID)",
			})
			return
		}

		provisioner, _, provisionerErr := getProvisionerForTenant(tenantID)
		if provisionerErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": provisionerErr.Error()})
			return
		}
		result, err := provisioner.Ensure(r.Context(), ProvisionInput{
			TenantID:       tenantID,
			ClientID:       clientID,
			ExternalUserID: externalUserID,
			DisplayName:    fmt.Sprintf("%s/%s/%s", tenantID, clientID, externalUserID),
		})
		if err != nil {
			log.Printf("provision-server ensure failed: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("/ingest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		if len(raw) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "empty ingest payload"})
			return
		}

		var payload map[string]any
		if err := json.Unmarshal(raw, &payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ingest json"})
			return
		}
		tenantID := stringField(payload, "tenant_id", "tenantId")
		if tenantID == "" {
			if dataValue, ok := payload["data"]; ok {
				if dataMap, ok := dataValue.(map[string]any); ok {
					tenantID = stringField(dataMap, "tenant_id", "tenantId")
				}
			}
		}
		if tenantID == "" {
			tenantID = stringField(payload, "subject")
			if parsedTenantID, _, _, parsed := parseAuthID(tenantID); parsed {
				tenantID = parsedTenantID
			} else {
				tenantID = strings.TrimSpace(defaultTenantID)
			}
		}
		if tenantID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tenant_id is required for ingest"})
			return
		}

		rewritten, rewriteErr := rewriteIngestPayload(payload)
		if rewriteErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": rewriteErr.Error()})
			return
		}
		raw = rewritten

		_, tenantToken, provisionerErr := getProvisionerForTenant(tenantID)
		if provisionerErr != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": provisionerErr.Error()})
			return
		}

		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, ingestURL, bytes.NewReader(raw))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create upstream request"})
			return
		}
		req.Header.Set("Content-Type", "application/cloudevents+json")
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(tenantToken))

		res, err := http.DefaultClient.Do(req)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream ingest failed"})
			return
		}
		defer res.Body.Close()
		responseBody, _ := io.ReadAll(res.Body)
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(res.StatusCode)
			if len(responseBody) == 0 {
				_, _ = w.Write([]byte(`{"error":"ingest upstream error"}`))
				return
			}
			_, _ = w.Write(responseBody)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNoContent)
	})

	addr := "127.0.0.1:" + port
	log.Printf("provision-server listening on %s plan=%s", addr, planKey)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
