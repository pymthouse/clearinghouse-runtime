package konnect

import (
	"context"
	"fmt"
	"strings"

	sdkkonnectgo "github.com/Kong/sdk-konnect-go"
	"github.com/Kong/sdk-konnect-go/models/components"
	"github.com/Kong/sdk-konnect-go/models/operations"
)

type EnsureOIDCSSOInput struct {
	IssuerURL      string
	ClientID       string
	ClientSecret   string
	LoginPath      string
	GroupClaimName string
}

func EnsureOIDCSSO(ctx context.Context, sdk *sdkkonnectgo.SDK, input EnsureOIDCSSOInput) (string, bool, error) {
	idp, err := getOIDCIdentityProvider(ctx, sdk)
	if err != nil {
		return "", false, err
	}

	updated := false
	if idp == nil {
		enabled := true
		identityType := components.IdentityProviderTypeOidc
		config := components.CreateCreateIdentityProviderConfigOIDCIdentityProviderConfig(components.OIDCIdentityProviderConfig{
			IssuerURL:    strings.TrimSpace(input.IssuerURL),
			ClientID:     strings.TrimSpace(input.ClientID),
			ClientSecret: sdkkonnectgo.Pointer(strings.TrimSpace(input.ClientSecret)),
			Scopes:       []string{"profile", "email"},
			ClaimMappings: &components.OIDCIdentityProviderClaimMappings{
				Groups: sdkkonnectgo.Pointer(strings.TrimSpace(input.GroupClaimName)),
			},
		})
		response, createErr := sdk.AuthSettings.CreateIdentityProvider(ctx, components.CreateIdentityProvider{
			Type:      &identityType,
			Enabled:   &enabled,
			LoginPath: sdkkonnectgo.Pointer(strings.TrimSpace(input.LoginPath)),
			Config:    &config,
		})
		if createErr != nil {
			return "", false, fmt.Errorf("create oidc identity provider: %w", createErr)
		}
		if response.IdentityProvider == nil || response.IdentityProvider.ID == nil {
			return "", false, fmt.Errorf("create oidc identity provider: empty response")
		}
		idp = response.IdentityProvider
		updated = true
	} else {
		enabled := true
		config := components.CreateUpdateIdentityProviderConfigOIDCIdentityProviderConfig(components.OIDCIdentityProviderConfig{
			IssuerURL:    strings.TrimSpace(input.IssuerURL),
			ClientID:     strings.TrimSpace(input.ClientID),
			ClientSecret: sdkkonnectgo.Pointer(strings.TrimSpace(input.ClientSecret)),
			Scopes:       []string{"profile", "email"},
			ClaimMappings: &components.OIDCIdentityProviderClaimMappings{
				Groups: sdkkonnectgo.Pointer(strings.TrimSpace(input.GroupClaimName)),
			},
		})
		idpID := strings.TrimSpace(*idp.ID)
		_, updateErr := sdk.AuthSettings.UpdateIdentityProvider(ctx, idpID, components.UpdateIdentityProvider{
			Enabled:   &enabled,
			LoginPath: sdkkonnectgo.Pointer(strings.TrimSpace(input.LoginPath)),
			Config:    &config,
		})
		if updateErr != nil {
			return "", false, fmt.Errorf("update oidc identity provider %s: %w", idpID, updateErr)
		}
		updated = true
	}

	if err := ensureAuthenticationSettings(ctx, sdk); err != nil {
		return "", false, err
	}
	return strings.TrimSpace(*idp.ID), updated, nil
}

func EnsureIDPTeamGroupMapping(
	ctx context.Context,
	sdk *sdkkonnectgo.SDK,
	idpID string,
	group string,
	teamID string,
) (string, bool, error) {
	existingID, err := findIDPTeamGroupMapping(ctx, sdk, idpID, group, teamID)
	if err != nil {
		return "", false, err
	}
	if existingID != "" {
		return existingID, false, nil
	}

	response, err := sdk.AuthSettings.CreateIdpTeamGroupMapping(
		ctx,
		strings.TrimSpace(idpID),
		components.CreateIdpTeamGroupMappingRequest{
			Group:  strings.TrimSpace(group),
			TeamID: strings.TrimSpace(teamID),
		},
	)
	if err != nil {
		return "", false, fmt.Errorf("create idp team-group mapping (%s -> %s): %w", group, teamID, err)
	}
	if response.IdpTeamGroupMapping == nil {
		return "", false, fmt.Errorf("create idp team-group mapping (%s -> %s): empty response", group, teamID)
	}
	return strings.TrimSpace(response.IdpTeamGroupMapping.ID), true, nil
}

func ensureAuthenticationSettings(ctx context.Context, sdk *sdkkonnectgo.SDK) error {
	valueTrue := true
	valueFalse := false
	_, err := sdk.AuthSettings.UpdateAuthenticationSettings(ctx, &components.UpdateAuthenticationSettings{
		OidcAuthEnabled:      &valueTrue,
		IdpMappingEnabled:    &valueTrue,
		KonnectMappingEnabled: &valueFalse,
	})
	if err != nil {
		return fmt.Errorf("enable oidc auth settings: %w", err)
	}
	return nil
}

func getOIDCIdentityProvider(ctx context.Context, sdk *sdkkonnectgo.SDK) (*components.IdentityProvider, error) {
	response, err := sdk.AuthSettings.GetIdentityProviders(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("list identity providers: %w", err)
	}
	if len(response.IdentityProviders) == 0 {
		return nil, nil
	}
	for _, idp := range response.IdentityProviders {
		if idp.Type != nil && *idp.Type == components.IdentityProviderTypeOidc {
			current := idp
			return &current, nil
		}
	}
	return nil, nil
}

func findIDPTeamGroupMapping(
	ctx context.Context,
	sdk *sdkkonnectgo.SDK,
	idpID string,
	group string,
	teamID string,
) (string, error) {
	pageSize := int64(100)
	filter := &operations.ListIdpTeamGroupMappingsQueryParamFilter{
		TeamID: &components.StringFieldEqualsFilter{
			Eq: sdkkonnectgo.Pointer(strings.TrimSpace(teamID)),
		},
		Group: &components.StringFieldEqualsFilter{
			Eq: sdkkonnectgo.Pointer(strings.TrimSpace(group)),
		},
	}
	response, err := sdk.AuthSettings.ListIdpTeamGroupMappings(ctx, operations.ListIdpTeamGroupMappingsRequest{
		IdpID:    strings.TrimSpace(idpID),
		PageSize: &pageSize,
		Filter:   filter,
	})
	if err != nil {
		return "", fmt.Errorf("list idp team-group mappings for %s: %w", idpID, err)
	}
	if response.IdpTeamGroupMappingsCollection == nil {
		return "", nil
	}
	for _, mapping := range response.IdpTeamGroupMappingsCollection.Data {
		if strings.TrimSpace(mapping.Group) == strings.TrimSpace(group) &&
			strings.TrimSpace(mapping.TeamID) == strings.TrimSpace(teamID) {
			return strings.TrimSpace(mapping.ID), nil
		}
	}
	return "", nil
}
