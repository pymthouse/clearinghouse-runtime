package config

import "strings"

const (
	// DefaultDockerWebhookURL is the in-compose identity webhook used by remote-signer.
	DefaultDockerWebhookURL = "http://identity-webhook:8090/authorize"
)

// ResolveRemoteSignerWebhookURL picks the webhook URL for go-livepeer -remoteSignerWebhookUrl.
// Explicit env wins, then PLATFORM_URL/webhooks/remote-signer, then the docker-compose default.
func ResolveRemoteSignerWebhookURL(platformURL, explicit string) string {
	if explicit != "" {
		return explicit
	}
	platformURL = strings.TrimSpace(platformURL)
	if platformURL != "" {
		return strings.TrimSuffix(platformURL, "/") + "/webhooks/remote-signer"
	}
	return DefaultDockerWebhookURL
}

// OpenMeterIngestURL returns the Konnect/OpenMeter events ingest endpoint.
func OpenMeterIngestURL(openmeterURL string) string {
	return strings.TrimSuffix(strings.TrimSpace(openmeterURL), "/") + "/events"
}
