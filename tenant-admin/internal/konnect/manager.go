package konnect

import (
	"context"
	"time"

	sdkkonnectgo "github.com/Kong/sdk-konnect-go"
)

type Manager struct {
	SDK *sdkkonnectgo.SDK
}

func (m *Manager) EnsureTeamByName(
	ctx context.Context,
	teamName string,
	description string,
	labels map[string]string,
) (string, bool, error) {
	return EnsureTeamByName(ctx, m.SDK, teamName, description, labels)
}

func (m *Manager) EnsureSystemAccountByName(ctx context.Context, name string, description string) (string, bool, error) {
	return EnsureSystemAccountByName(ctx, m.SDK, name, description)
}

func (m *Manager) CreateSystemAccountToken(
	ctx context.Context,
	accountID string,
	tokenName string,
	expiresAt time.Time,
) (*SystemAccountToken, error) {
	return CreateSystemAccountToken(ctx, m.SDK, accountID, tokenName, expiresAt)
}

func (m *Manager) DeleteSystemAccountToken(ctx context.Context, accountID string, tokenID string) error {
	return DeleteSystemAccountToken(ctx, m.SDK, accountID, tokenID)
}

func (m *Manager) EnsureSystemAccountInTeam(ctx context.Context, teamID string, accountID string) error {
	return EnsureSystemAccountInTeam(ctx, m.SDK, teamID, accountID)
}

func (m *Manager) EnsureRoleAssignedToSystemAccount(ctx context.Context, accountID string, roleName string) error {
	return EnsureRoleAssignedToSystemAccount(ctx, m.SDK, accountID, roleName)
}
