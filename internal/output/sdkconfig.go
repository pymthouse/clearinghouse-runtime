package output

import (
	"encoding/json"
	"strings"

	"github.com/livepeer/clearinghouse/internal/auth0"
	"github.com/livepeer/clearinghouse/internal/config"
)

type SDKConfig struct {
	Auth0        SDKAuth0        `json:"auth0"`
	Signer       SDKSigner       `json:"signer"`
	RemoteSigner SDKRemoteSigner `json:"remoteSigner"`
	OpenMeter    *SDKOpenMeter   `json:"openmeter,omitempty"`
}

type SDKAuth0 struct {
	Domain   string `json:"domain"`
	Issuer   string `json:"issuer"`
	JwksURL  string `json:"jwksUrl"`
	ClientID string `json:"clientId"`
	Audience string `json:"audience"`
}

type SDKSigner struct {
	ProxyURL  string `json:"proxyUrl"`
	PublicURL string `json:"publicUrl"`
	Audience  string `json:"audience"`
}

type SDKRemoteSigner struct {
	WebhookURL string `json:"webhookUrl"`
}

type SDKOpenMeter struct {
	URL             string `json:"url"`
	TrialFeatureKey string `json:"trialFeatureKey"`
}

func BuildSDKConfig(cfg *config.BootstrapConfig, auth0Result *auth0.ProvisionResult) ([]byte, error) {
	sdkCfg := SDKConfig{
		Auth0: SDKAuth0{
			Domain:   cfg.Auth0Domain,
			Issuer:   auth0Result.Issuer,
			JwksURL:  auth0Result.JwksURL,
			ClientID: auth0Result.PublicClientID,
			Audience: auth0Result.APIIdentifier,
		},
		Signer: SDKSigner{
			ProxyURL:  cfg.SignerProxyURL,
			PublicURL: cfg.SignerPublicURL,
			Audience:  auth0Result.APIIdentifier,
		},
		RemoteSigner: SDKRemoteSigner{
			WebhookURL: cfg.RemoteSignerWebhookURL,
		},
	}

	if !cfg.SkipOpenMeter {
		url := strings.TrimSuffix(cfg.OpenmeterURL, "/")
		sdkCfg.OpenMeter = &SDKOpenMeter{
			URL:             url,
			TrialFeatureKey: cfg.TrialFeatureKey,
		}
	}

	return json.MarshalIndent(sdkCfg, "", "  ")
}
