package admin

import "strings"

func CreateAdmin(baseURL, apiKey string) OpenMeterAdmin {
	url := strings.TrimSpace(baseURL)
	key := strings.TrimSpace(apiKey)

	// Normalize to API root (strip /openmeter and /events suffixes)
	url = strings.TrimSuffix(url, "/")
	url = strings.TrimSuffix(url, "/events")
	url = strings.TrimSuffix(url, "/openmeter")

	// Strip /v3 — the SDK adds its own base path
	url = strings.TrimSuffix(url, "/v3")

	return NewKonnectAdmin(KonnectAdminConfig{
		APIKey:  key,
		BaseURL: url,
	})
}
