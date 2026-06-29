package konnect

import (
	"context"
	"fmt"
	"strings"
	"time"

	sdkkonnectgo "github.com/Kong/sdk-konnect-go"
	"github.com/Kong/sdk-konnect-go/models/components"
	"github.com/Kong/sdk-konnect-go/models/operations"
)

type SystemAccountToken struct {
	AccountID string
	TokenID   string
	Token     string
	ExpiresAt time.Time
}

func EnsureSystemAccountByName(
	ctx context.Context,
	sdk *sdkkonnectgo.SDK,
	name string,
	description string,
) (string, bool, error) {
	existingID, err := findSystemAccountIDByName(ctx, sdk, name)
	if err != nil {
		return "", false, err
	}
	if existingID != "" {
		return existingID, false, nil
	}

	response, err := sdk.SystemAccounts.PostSystemAccounts(ctx, &components.CreateSystemAccount{
		Name:        strings.TrimSpace(name),
		Description: strings.TrimSpace(description),
	})
	if err != nil {
		existingID, findErr := findSystemAccountIDByName(ctx, sdk, name)
		if findErr == nil && existingID != "" {
			return existingID, false, nil
		}
		return "", false, fmt.Errorf("create system account %q: %w", name, err)
	}
	if response.SystemAccount == nil || response.SystemAccount.ID == nil || strings.TrimSpace(*response.SystemAccount.ID) == "" {
		return "", false, fmt.Errorf("create system account %q: empty response", name)
	}
	return strings.TrimSpace(*response.SystemAccount.ID), true, nil
}

func CreateSystemAccountToken(
	ctx context.Context,
	sdk *sdkkonnectgo.SDK,
	accountID string,
	tokenName string,
	expiresAt time.Time,
) (*SystemAccountToken, error) {
	response, err := sdk.SystemAccountsAccessTokens.PostSystemAccountsIDAccessTokens(
		ctx,
		strings.TrimSpace(accountID),
		&components.CreateSystemAccountAccessToken{
			Name:      strings.TrimSpace(tokenName),
			ExpiresAt: expiresAt.UTC(),
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create system account token: %w", err)
	}
	if response.SystemAccountAccessTokenCreated == nil ||
		response.SystemAccountAccessTokenCreated.ID == nil ||
		response.SystemAccountAccessTokenCreated.Token == nil {
		return nil, fmt.Errorf("create system account token: empty response")
	}

	return &SystemAccountToken{
		AccountID: strings.TrimSpace(accountID),
		TokenID:   strings.TrimSpace(*response.SystemAccountAccessTokenCreated.ID),
		Token:     strings.TrimSpace(*response.SystemAccountAccessTokenCreated.Token),
		ExpiresAt: expiresAt.UTC(),
	}, nil
}

func DeleteSystemAccountToken(
	ctx context.Context,
	sdk *sdkkonnectgo.SDK,
	accountID string,
	tokenID string,
) error {
	_, err := sdk.SystemAccountsAccessTokens.DeleteSystemAccountsIDAccessTokensID(
		ctx,
		strings.TrimSpace(accountID),
		strings.TrimSpace(tokenID),
	)
	if err != nil {
		return fmt.Errorf("delete system account token %s: %w", tokenID, err)
	}
	return nil
}

func EnsureSystemAccountInTeam(
	ctx context.Context,
	sdk *sdkkonnectgo.SDK,
	teamID string,
	accountID string,
) error {
	inTeam, err := systemAccountInTeam(ctx, sdk, teamID, accountID)
	if err != nil {
		return err
	}
	if inTeam {
		return nil
	}

	_, err = sdk.SystemAccountsTeamMembership.PostTeamsTeamIDSystemAccounts(
		ctx,
		strings.TrimSpace(teamID),
		&components.AddSystemAccountToTeam{
			AccountID: sdkkonnectgo.Pointer(strings.TrimSpace(accountID)),
		},
	)
	if err != nil {
		return fmt.Errorf("add system account %s to team %s: %w", accountID, teamID, err)
	}
	return nil
}

func EnsureRoleAssignedToSystemAccount(
	ctx context.Context,
	sdk *sdkkonnectgo.SDK,
	accountID string,
	roleName string,
) error {
	assigned, err := systemAccountHasRole(ctx, sdk, accountID, roleName)
	if err != nil {
		return err
	}
	if assigned {
		return nil
	}

	// SECURITY GATE: this assigns the role with wildcard entity scope (entityID "*",
	// EntityRegion Wildcard) over the "Metering" entity type, i.e. org-wide. That is
	// acceptable for the per-tenant *Ingest* role this service assigns today, because
	// ingestion is write-only event submission. It is NOT safe for read roles such as
	// "Metering Viewer" / "Billing Viewer": a wildcard read scope would expose every
	// tenant's metering/billing data. Reader tokens are therefore intentionally gated
	// (see httpapi token handler) until we confirm Konnect can scope Metering/Billing
	// roles to a per-tenant resource boundary. Do not reuse this helper for read roles
	// without first narrowing entityID/entityType to the tenant's actual resources.
	role := components.RoleName(strings.TrimSpace(roleName))
	entityID := "*"
	entityRegion := components.AssignRoleEntityRegionWildcard
	entityType := components.EntityTypeName("Metering")
	assignRole := &components.AssignRole{
		RoleName:       &role,
		EntityID:       &entityID,
		EntityTypeName: &entityType,
		EntityRegion:   &entityRegion,
	}
	_, err = sdk.SystemAccountsRoles.PostSystemAccountsAccountIDAssignedRoles(ctx, strings.TrimSpace(accountID), assignRole)
	if err != nil {
		return fmt.Errorf("assign role %q to system account %s: %w", roleName, accountID, err)
	}
	return nil
}

func findSystemAccountIDByName(ctx context.Context, sdk *sdkkonnectgo.SDK, name string) (string, error) {
	target := strings.TrimSpace(name)
	pageNumber := int64(1)
	pageSize := int64(100)
	filter := &operations.GetSystemAccountsQueryParamFilter{
		Name: &components.LegacyStringFieldFilter{
			Eq: &target,
		},
	}

	for {
		response, err := sdk.SystemAccounts.GetSystemAccounts(ctx, operations.GetSystemAccountsRequest{
			PageNumber: &pageNumber,
			PageSize:   &pageSize,
			Filter:     filter,
		})
		if err != nil {
			return "", fmt.Errorf("list system accounts for %q: %w", name, err)
		}
		if response.SystemAccountCollection == nil || len(response.SystemAccountCollection.Data) == 0 {
			return "", nil
		}
		for _, account := range response.SystemAccountCollection.Data {
			if account.Name != nil && strings.EqualFold(strings.TrimSpace(*account.Name), target) && account.ID != nil {
				return strings.TrimSpace(*account.ID), nil
			}
		}
		if int64(len(response.SystemAccountCollection.Data)) < pageSize {
			return "", nil
		}
		pageNumber++
	}
}

func systemAccountInTeam(ctx context.Context, sdk *sdkkonnectgo.SDK, teamID string, accountID string) (bool, error) {
	pageNumber := int64(1)
	pageSize := int64(100)
	for {
		response, err := sdk.SystemAccountsTeamMembership.GetTeamsTeamIDSystemAccounts(ctx, operations.GetTeamsTeamIDSystemAccountsRequest{
			TeamID:     strings.TrimSpace(teamID),
			PageNumber: &pageNumber,
			PageSize:   &pageSize,
		})
		if err != nil {
			return false, fmt.Errorf("list system accounts for team %s: %w", teamID, err)
		}
		if response.SystemAccountCollection == nil {
			return false, nil
		}
		for _, account := range response.SystemAccountCollection.Data {
			if account.ID != nil && strings.TrimSpace(*account.ID) == strings.TrimSpace(accountID) {
				return true, nil
			}
		}
		if int64(len(response.SystemAccountCollection.Data)) < pageSize {
			return false, nil
		}
		pageNumber++
	}
}

func systemAccountHasRole(ctx context.Context, sdk *sdkkonnectgo.SDK, accountID string, roleName string) (bool, error) {
	response, err := sdk.SystemAccountsRoles.GetSystemAccountsAccountIDAssignedRoles(ctx, strings.TrimSpace(accountID), nil)
	if err != nil {
		return false, fmt.Errorf("list assigned roles for system account %s: %w", accountID, err)
	}
	if response.AssignedRoleCollection == nil {
		return false, nil
	}
	for _, role := range response.AssignedRoleCollection.Data {
		if role.RoleName != nil && strings.EqualFold(strings.TrimSpace(*role.RoleName), strings.TrimSpace(roleName)) {
			return true, nil
		}
	}
	return false, nil
}
