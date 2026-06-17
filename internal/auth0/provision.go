package auth0

import (
	"context"
	"fmt"

	"github.com/auth0/go-auth0/v2/management"
	auth0client "github.com/auth0/go-auth0/v2/management/client"
	"github.com/auth0/go-auth0/v2/management/option"
)

type ProvisionResult struct {
	APIIdentifier  string
	PublicClientID string
	M2MClientID    string
	M2MClientSecret string
	JwksURL        string
	Issuer         string
}

type ProvisionConfig struct {
	Domain          string
	MgmtClientID    string
	MgmtClientSecret string
	AppName         string
	APIAudience     string
}

func Provision(ctx context.Context, cfg ProvisionConfig) (*ProvisionResult, error) {
	mgmt, err := auth0client.New(
		cfg.Domain,
		option.WithClientCredentials(ctx, cfg.MgmtClientID, cfg.MgmtClientSecret),
	)
	if err != nil {
		return nil, fmt.Errorf("creating Auth0 management client: %w", err)
	}

	// 1. Create resource server
	rsName := fmt.Sprintf("%s Livepeer API", cfg.AppName)
	signingAlg := management.SigningAlgorithmEnumRs256
	tokenLifetime := 86400
	skipConsent := true
	_, err = mgmt.ResourceServers.Create(ctx, &management.CreateResourceServerRequestContent{
		Name:       &rsName,
		Identifier: cfg.APIAudience,
		SigningAlg: &signingAlg,
		TokenLifetime: &tokenLifetime,
		SkipConsentForVerifiableFirstPartyClients: &skipConsent,
		Scopes: []*management.ResourceServerScope{
			{
				Value:       "sign:job",
				Description: strPtr("Sign payment tickets for Livepeer remote signer"),
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("creating resource server: %w", err)
	}

	// 2. Create public client (native, device_code + refresh_token)
	publicName := fmt.Sprintf("%s Public", cfg.AppName)
	publicDesc := "Public client for SDK, CLI, and device authorization flow"
	appTypeNative := management.ClientAppTypeEnumNative
	authMethodNone := management.ClientTokenEndpointAuthMethodEnumNone
	isFirstParty := true
	oidcConformant := true

	publicResp, err := mgmt.Clients.Create(ctx, &management.CreateClientRequestContent{
		Name:                   publicName,
		Description:            &publicDesc,
		AppType:                &appTypeNative,
		OidcConformant:         &oidcConformant,
		IsFirstParty:           &isFirstParty,
		GrantTypes:             []string{"urn:ietf:params:oauth:grant-type:device_code", "refresh_token"},
		TokenEndpointAuthMethod: &authMethodNone,
	})
	if err != nil {
		return nil, fmt.Errorf("creating public client: %w", err)
	}
	publicClientID := derefStr(publicResp.ClientID)
	if publicClientID == "" {
		return nil, fmt.Errorf("Auth0 public client missing client_id")
	}

	// 3. Create M2M client (non_interactive, client_credentials)
	m2mName := fmt.Sprintf("%s M2M", cfg.AppName)
	m2mDesc := "Confidential client for server-side operations"
	appTypeM2M := management.ClientAppTypeEnumNonInteractive
	authMethodPost := management.ClientTokenEndpointAuthMethodEnumClientSecretPost

	m2mResp, err := mgmt.Clients.Create(ctx, &management.CreateClientRequestContent{
		Name:                   m2mName,
		Description:            &m2mDesc,
		AppType:                &appTypeM2M,
		OidcConformant:         &oidcConformant,
		GrantTypes:             []string{"client_credentials"},
		TokenEndpointAuthMethod: &authMethodPost,
	})
	if err != nil {
		return nil, fmt.Errorf("creating M2M client: %w", err)
	}
	m2mClientID := derefStr(m2mResp.ClientID)
	m2mClientSecret := derefStr(m2mResp.ClientSecret)
	if m2mClientID == "" {
		return nil, fmt.Errorf("Auth0 M2M client missing client_id")
	}
	if m2mClientSecret == "" {
		return nil, fmt.Errorf("Auth0 M2M client missing client_secret")
	}

	// 4. Create client grants
	_, err = mgmt.ClientGrants.Create(ctx, &management.CreateClientGrantRequestContent{
		ClientID: &m2mClientID,
		Audience: cfg.APIAudience,
		Scope:    []string{"sign:job"},
	})
	if err != nil {
		return nil, fmt.Errorf("creating M2M client grant: %w", err)
	}

	_, err = mgmt.ClientGrants.Create(ctx, &management.CreateClientGrantRequestContent{
		ClientID: &publicClientID,
		Audience: cfg.APIAudience,
		Scope:    []string{"sign:job"},
	})
	if err != nil {
		return nil, fmt.Errorf("creating public client grant: %w", err)
	}

	issuer := fmt.Sprintf("https://%s/", cfg.Domain)
	jwksURL := fmt.Sprintf("https://%s/.well-known/jwks.json", cfg.Domain)

	return &ProvisionResult{
		APIIdentifier:  cfg.APIAudience,
		PublicClientID: publicClientID,
		M2MClientID:    m2mClientID,
		M2MClientSecret: m2mClientSecret,
		JwksURL:        jwksURL,
		Issuer:         issuer,
	}, nil
}

func strPtr(s string) *string { return &s }

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
