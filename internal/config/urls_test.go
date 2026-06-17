package config

import "testing"

func TestResolveRemoteSignerWebhookURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		platform   string
		explicit   string
		want       string
	}{
		{
			name:     "explicit wins",
			platform: "https://app.vercel.app",
			explicit: "https://custom.example/authorize",
			want:     "https://custom.example/authorize",
		},
		{
			name:     "platform url",
			platform: "https://app.vercel.app/",
			want:     "https://app.vercel.app/webhooks/remote-signer",
		},
		{
			name: "docker default",
			want: DefaultDockerWebhookURL,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveRemoteSignerWebhookURL(tc.platform, tc.explicit)
			if got != tc.want {
				t.Errorf("ResolveRemoteSignerWebhookURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestOpenMeterIngestURL(t *testing.T) {
	t.Parallel()

	got := OpenMeterIngestURL("https://us.api.konghq.com/v3/openmeter/")
	want := "https://us.api.konghq.com/v3/openmeter/events"
	if got != want {
		t.Errorf("OpenMeterIngestURL() = %q, want %q", got, want)
	}
}
