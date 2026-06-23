package auth0

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	domain       string
	clientID     string
	clientSecret string
	httpClient   *http.Client
}

type EnsureTenantInput struct {
	TenantSlug      string
	TenantName      string
	AdminEmails     []string
	DefaultPassword string
	ConnectionName  string
}

type EnsureTenantResult struct {
	OrganizationID string
	AdminUserIDs   []string
}

func NewClient(domain, clientID, clientSecret string) *Client {
	return &Client{
		domain:       strings.TrimSpace(domain),
		clientID:     strings.TrimSpace(clientID),
		clientSecret: strings.TrimSpace(clientSecret),
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *Client) EnsureTenant(ctx context.Context, input EnsureTenantInput) (*EnsureTenantResult, error) {
	token, err := c.managementToken(ctx)
	if err != nil {
		return nil, err
	}

	orgID, err := c.ensureOrganization(ctx, token, input.TenantSlug, input.TenantName)
	if err != nil {
		return nil, err
	}
	if err := c.ensureOrganizationConnection(ctx, token, orgID, input.ConnectionName); err != nil {
		return nil, err
	}

	adminUserIDs := make([]string, 0, len(input.AdminEmails))
	for _, email := range input.AdminEmails {
		trimmedEmail := strings.TrimSpace(strings.ToLower(email))
		if trimmedEmail == "" {
			continue
		}
		userID, ensureErr := c.ensureUser(ctx, token, trimmedEmail, input.DefaultPassword, input.ConnectionName)
		if ensureErr != nil {
			return nil, ensureErr
		}
		adminUserIDs = append(adminUserIDs, userID)
		if err := c.ensureUserInOrganization(ctx, token, orgID, userID); err != nil {
			return nil, err
		}
	}

	return &EnsureTenantResult{
		OrganizationID: orgID,
		AdminUserIDs:   adminUserIDs,
	}, nil
}

func (c *Client) managementToken(ctx context.Context) (string, error) {
	body := map[string]string{
		"client_id":     c.clientID,
		"client_secret": c.clientSecret,
		"audience":      fmt.Sprintf("https://%s/api/v2/", c.domain),
		"grant_type":    "client_credentials",
	}
	responseBody, err := c.doJSON(ctx, http.MethodPost, c.apiURL("/oauth/token"), "", body)
	if err != nil {
		return "", fmt.Errorf("auth0 management token: %w", err)
	}
	var parsed struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return "", fmt.Errorf("auth0 management token decode: %w", err)
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return "", fmt.Errorf("auth0 management token: empty access_token")
	}
	return parsed.AccessToken, nil
}

func (c *Client) ensureOrganization(ctx context.Context, token, slug, name string) (string, error) {
	searchURL := c.apiURL("/api/v2/organizations")
	responseBody, err := c.doJSON(ctx, http.MethodGet, searchURL, token, nil)
	if err != nil {
		return "", fmt.Errorf("list organizations: %w", err)
	}
	var organizations []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(responseBody, &organizations); err != nil {
		return "", fmt.Errorf("decode organizations: %w", err)
	}
	targetSlug := strings.TrimSpace(strings.ToLower(slug))
	for _, org := range organizations {
		if strings.TrimSpace(strings.ToLower(org.Name)) == targetSlug && strings.TrimSpace(org.ID) != "" {
			return strings.TrimSpace(org.ID), nil
		}
	}

	payload := map[string]any{
		"name":         strings.TrimSpace(slug),
		"display_name": strings.TrimSpace(name),
	}
	createBody, err := c.doJSON(ctx, http.MethodPost, c.apiURL("/api/v2/organizations"), token, payload)
	if err != nil {
		return "", fmt.Errorf("create organization: %w", err)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(createBody, &created); err != nil {
		return "", fmt.Errorf("decode created organization: %w", err)
	}
	if strings.TrimSpace(created.ID) == "" {
		return "", fmt.Errorf("create organization: missing id")
	}
	return strings.TrimSpace(created.ID), nil
}

func (c *Client) ensureOrganizationConnection(ctx context.Context, token, orgID, connectionName string) error {
	connectionID, err := c.findConnectionIDByName(ctx, token, connectionName)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"connection_id": strings.TrimSpace(connectionID),
		"assign_membership_on_login": true,
	}
	_, err = c.doJSON(ctx, http.MethodPost, c.apiURL(fmt.Sprintf("/api/v2/organizations/%s/enabled_connections", url.PathEscape(orgID))), token, payload)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "already exists") {
		return fmt.Errorf("enable org connection: %w", err)
	}
	return nil
}

func (c *Client) findConnectionIDByName(ctx context.Context, token, connectionName string) (string, error) {
	searchURL := c.apiURL("/api/v2/connections?name=" + url.QueryEscape(strings.TrimSpace(connectionName)))
	responseBody, err := c.doJSON(ctx, http.MethodGet, searchURL, token, nil)
	if err != nil {
		return "", fmt.Errorf("list connections: %w", err)
	}
	var connections []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(responseBody, &connections); err != nil {
		return "", fmt.Errorf("decode connections: %w", err)
	}
	targetName := strings.TrimSpace(strings.ToLower(connectionName))
	for _, connection := range connections {
		if strings.TrimSpace(strings.ToLower(connection.Name)) == targetName && strings.TrimSpace(connection.ID) != "" {
			return strings.TrimSpace(connection.ID), nil
		}
	}
	return "", fmt.Errorf("auth0 connection %q not found", connectionName)
}

func (c *Client) ensureUser(ctx context.Context, token, email, password, connectionName string) (string, error) {
	searchURL := c.apiURL("/api/v2/users-by-email?email=" + url.QueryEscape(email))
	responseBody, err := c.doJSON(ctx, http.MethodGet, searchURL, token, nil)
	if err != nil {
		return "", fmt.Errorf("list users by email %q: %w", email, err)
	}
	var users []struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(responseBody, &users); err != nil {
		return "", fmt.Errorf("decode users by email: %w", err)
	}
	if len(users) > 0 && strings.TrimSpace(users[0].UserID) != "" {
		return strings.TrimSpace(users[0].UserID), nil
	}

	payload := map[string]any{
		"connection": strings.TrimSpace(connectionName),
		"email":      email,
		"password":   strings.TrimSpace(password),
	}
	createBody, err := c.doJSON(ctx, http.MethodPost, c.apiURL("/api/v2/users"), token, payload)
	if err != nil {
		return "", fmt.Errorf("create user %q: %w", email, err)
	}
	var created struct {
		UserID string `json:"user_id"`
	}
	if err := json.Unmarshal(createBody, &created); err != nil {
		return "", fmt.Errorf("decode created user: %w", err)
	}
	if strings.TrimSpace(created.UserID) == "" {
		return "", fmt.Errorf("create user %q: missing user_id", email)
	}
	return strings.TrimSpace(created.UserID), nil
}

func (c *Client) ensureUserInOrganization(ctx context.Context, token, orgID, userID string) error {
	payload := map[string]any{
		"members": []string{strings.TrimSpace(userID)},
	}
	_, err := c.doJSON(ctx, http.MethodPost, c.apiURL(fmt.Sprintf("/api/v2/organizations/%s/members", url.PathEscape(orgID))), token, payload)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "members already exist") {
		return fmt.Errorf("add user %s to organization %s: %w", userID, orgID, err)
	}
	return nil
}

func (c *Client) apiURL(path string) string {
	return fmt.Sprintf("https://%s%s", c.domain, path)
}

func (c *Client) doJSON(ctx context.Context, method, rawURL, token string, payload any) ([]byte, error) {
	var bodyReader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, rawURL, bodyReader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if strings.TrimSpace(token) != "" {
		request.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", response.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return responseBody, nil
}
