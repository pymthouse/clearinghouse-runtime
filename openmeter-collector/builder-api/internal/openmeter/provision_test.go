package openmeter_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/livepeer/clearinghouse/openmeter-collector/builder-api/internal/openmeter"
)

func TestCustomerKey(t *testing.T) {
	t.Parallel()
	got := openmeter.CustomerKey(" pub-client ", " demo-user ")
	if got != "pub-client:demo-user" {
		t.Fatalf("CustomerKey() = %q", got)
	}
}

func TestProvisionSessionCreatesCustomerSubscriptionAndGrant(t *testing.T) {
	t.Parallel()

	var (
		customerCreated bool
		subscriptionCreated bool
		grantCreated bool
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/customers":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/customers":
			customerCreated = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":"cust-1","key":"pub-client:demo-user"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/subscriptions":
			w.Header().Set("Content-Type", "application/json")
			if subscriptionCreated {
				_, _ = w.Write([]byte(`{"data":[{"id":"sub-1","customer_id":"cust-1","status":"active"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"data":[]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/subscriptions":
			subscriptionCreated = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"sub-1"}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/entitlements/network_spend/value"):
			w.Header().Set("Content-Type", "application/json")
			if grantCreated {
				_, _ = w.Write([]byte(`{"hasAccess":true,"balance":5000000,"usage":0,"totalAvailableGrantAmount":5000000}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
			return
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/entitlements/network_spend/grants"):
			grantCreated = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"grant-1"}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client := openmeter.New(server.URL, "test-key")
	result, err := client.ProvisionSession(context.Background(), openmeter.ProvisionConfig{
		DefaultPlanKey:               "clearinghouse_default_ppu",
		TrialFeatureKey:              "network_spend",
		DefaultStarterIncludedMicros: 5_000_000,
	}, "pub-client", "demo-user")
	if err != nil {
		t.Fatal(err)
	}
	if result.CustomerKey != "pub-client:demo-user" {
		t.Fatalf("customer key = %q", result.CustomerKey)
	}
	if !customerCreated || !subscriptionCreated || !grantCreated {
		t.Fatalf("customer=%v subscription=%v grant=%v", customerCreated, subscriptionCreated, grantCreated)
	}
	if !result.Balance.HasAccess {
		t.Fatalf("expected access, got %+v", result.Balance)
	}
	if result.Balance.BalanceUsdMicros != "5000000" {
		t.Fatalf("balance = %q", result.Balance.BalanceUsdMicros)
	}
}

func TestProvisionSessionReusesExistingCustomer(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/customers":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"cust-1","key":"pub-client:demo-user"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/subscriptions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"data":[{"id":"sub-1","customer_id":"cust-1","status":"active"}]}`))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/entitlements/network_spend/value"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"hasAccess":true,"balance":1000000,"usage":4000000,"totalAvailableGrantAmount":5000000}`))
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(server.Close)

	client := openmeter.New(server.URL, "test-key")
	result, err := client.ProvisionSession(context.Background(), openmeter.ProvisionConfig{
		DefaultPlanKey:               "clearinghouse_default_ppu",
		TrialFeatureKey:              "network_spend",
		DefaultStarterIncludedMicros: 5_000_000,
	}, "pub-client", "demo-user")
	if err != nil {
		t.Fatal(err)
	}
	if result.Balance.BalanceUsdMicros != "1000000" {
		t.Fatalf("balance = %q", result.Balance.BalanceUsdMicros)
	}
}

func TestGetTrialCreditBalanceFallsBackToConfiguredGrant(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(server.Close)

	client := openmeter.New(server.URL, "test-key")
	balance, err := client.GetTrialCreditBalanceWithFallback(context.Background(), "pub-client:demo-user", "network_spend", 5_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if !balance.HasAccess || balance.BalanceUsdMicros != "5000000" {
		t.Fatalf("balance = %+v", balance)
	}
}

func TestGetTrialCreditBalanceExhausted(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"hasAccess":                 false,
			"balance":                   0,
			"usage":                     5_000_000,
			"totalAvailableGrantAmount": 5_000_000,
		})
	}))
	t.Cleanup(server.Close)

	client := openmeter.New(server.URL, "test-key")
	balance, err := client.GetTrialCreditBalance(context.Background(), "pub-client:demo-user", "network_spend", 5_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if balance.HasAccess {
		t.Fatalf("expected exhausted balance, got %+v", balance)
	}
}
