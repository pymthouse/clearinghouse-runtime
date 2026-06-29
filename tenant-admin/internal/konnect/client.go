package konnect

import (
	"strings"

	sdkkonnectgo "github.com/Kong/sdk-konnect-go"
	"github.com/Kong/sdk-konnect-go/models/components"
)

func NewSDK(baseURL, token string) *sdkkonnectgo.SDK {
	url := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	options := []sdkkonnectgo.SDKOption{
		sdkkonnectgo.WithSecurity(components.Security{
			PersonalAccessToken: sdkkonnectgo.Pointer(strings.TrimSpace(token)),
		}),
	}
	if url != "" {
		options = append(options, sdkkonnectgo.WithServerURL(url))
	}
	return sdkkonnectgo.New(options...)
}
