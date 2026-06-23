package konnect

import (
	"context"
	"fmt"
	"strings"

	sdkkonnectgo "github.com/Kong/sdk-konnect-go"
	"github.com/Kong/sdk-konnect-go/models/components"
	"github.com/Kong/sdk-konnect-go/models/operations"
)

func EnsureTeamByName(
	ctx context.Context,
	sdk *sdkkonnectgo.SDK,
	teamName string,
	description string,
	labels map[string]string,
) (string, bool, error) {
	existingID, err := findTeamIDByName(ctx, sdk, teamName)
	if err != nil {
		return "", false, err
	}
	if existingID != "" {
		return existingID, false, nil
	}

	request := &components.CreateTeam{
		Name:   strings.TrimSpace(teamName),
		Labels: labels,
	}
	if strings.TrimSpace(description) != "" {
		request.Description = sdkkonnectgo.Pointer(strings.TrimSpace(description))
	}

	response, err := sdk.Teams.CreateTeam(ctx, request)
	if err != nil {
		existingID, findErr := findTeamIDByName(ctx, sdk, teamName)
		if findErr == nil && existingID != "" {
			return existingID, false, nil
		}
		return "", false, fmt.Errorf("create team %q: %w", teamName, err)
	}
	if response.Team == nil || response.Team.ID == nil || strings.TrimSpace(*response.Team.ID) == "" {
		return "", false, fmt.Errorf("create team %q: empty response", teamName)
	}
	return strings.TrimSpace(*response.Team.ID), true, nil
}

func findTeamIDByName(ctx context.Context, sdk *sdkkonnectgo.SDK, teamName string) (string, error) {
	target := strings.TrimSpace(teamName)
	if target == "" {
		return "", nil
	}
	pageNumber := int64(1)
	pageSize := int64(100)
	filter := &operations.ListTeamsQueryParamFilter{
		Name: &components.LegacyStringFieldFilter{
			Eq: sdkkonnectgo.Pointer(target),
		},
	}

	for {
		response, err := sdk.Teams.ListTeams(ctx, operations.ListTeamsRequest{
			PageNumber: &pageNumber,
			PageSize:   &pageSize,
			Filter:     filter,
		})
		if err != nil {
			return "", fmt.Errorf("list teams for %q: %w", teamName, err)
		}
		if response.TeamCollection == nil || len(response.TeamCollection.Data) == 0 {
			return "", nil
		}
		for _, team := range response.TeamCollection.Data {
			if team.Name != nil && strings.EqualFold(strings.TrimSpace(*team.Name), target) && team.ID != nil {
				return strings.TrimSpace(*team.ID), nil
			}
		}
		if int64(len(response.TeamCollection.Data)) < pageSize {
			return "", nil
		}
		pageNumber++
	}
}
