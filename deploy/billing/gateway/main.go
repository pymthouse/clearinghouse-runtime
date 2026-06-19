package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/livepeer/clearinghouse/deploy/billing/balance"
	"github.com/livepeer/clearinghouse/deploy/billing/internal/auth"
	"github.com/livepeer/clearinghouse/deploy/billing/provision"
)

func requiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func main() {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8092"
	}
	planKey := strings.TrimSpace(os.Getenv("OPENMETER_DEFAULT_PLAN_KEY"))
	if planKey == "" {
		planKey = "clearinghouse_default_ppu"
	}
	featureKey := strings.TrimSpace(os.Getenv("OPENMETER_BALANCE_FEATURE_KEY"))
	if featureKey == "" {
		featureKey = "billable_spend"
	}

	baseURL, err := requiredEnv("OPENMETER_URL")
	if err != nil {
		log.Fatal(err)
	}
	apiKey, err := requiredEnv("OPENMETER_API_KEY")
	if err != nil {
		log.Fatal(err)
	}
	gatewaySecret, err := requiredEnv("BILLING_GATEWAY_SECRET")
	if err != nil {
		log.Fatal(err)
	}

	provisioner := provision.NewProvisioner(baseURL, apiKey, planKey)
	checker := balance.NewChecker(provisioner, baseURL, apiKey, featureKey)

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
		if !auth.AuthenticateBearer(r, gatewaySecret) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		var body provision.EnsureRequest
		if len(raw) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request json"})
				return
			}
		}

		clientID, externalUserID, ok := provision.ParseIdentity(body)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "client_id and external_user_id (or auth_id) are required",
			})
			return
		}

		result, err := provisioner.Ensure(r.Context(), provision.ProvisionInput{
			ClientID:       clientID,
			ExternalUserID: externalUserID,
			DisplayName:    fmt.Sprintf("%s:%s", clientID, externalUserID),
		})
		if err != nil {
			log.Printf("billing-gateway ensure failed: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	mux.HandleFunc("/balance", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if !auth.AuthenticateBearer(r, gatewaySecret) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}

		clientID := strings.TrimSpace(r.URL.Query().Get("clientId"))
		if clientID == "" {
			clientID = strings.TrimSpace(r.URL.Query().Get("client_id"))
		}
		externalUserID := strings.TrimSpace(r.URL.Query().Get("externalUserId"))
		if externalUserID == "" {
			externalUserID = strings.TrimSpace(r.URL.Query().Get("external_user_id"))
		}
		if clientID == "" || externalUserID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{
				"error": "clientId and externalUserId are required",
			})
			return
		}

		result, err := checker.Check(r.Context(), clientID, externalUserID)
		if err != nil {
			log.Printf("billing-gateway balance failed: %v", err)
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	})

	addr := "0.0.0.0:" + port
	log.Printf("billing-gateway listening on %s plan=%s feature=%s", addr, planKey, featureKey)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
