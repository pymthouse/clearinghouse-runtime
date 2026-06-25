package openmeter

import "testing"

func TestNormalizeOpenMeterURL(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "openmeter endpoint",
			input: "https://us.api.konghq.com/v3/openmeter",
			want:  "https://us.api.konghq.com",
		},
		{
			name:  "events endpoint",
			input: "https://us.api.konghq.com/v3/openmeter/events",
			want:  "https://us.api.konghq.com",
		},
		{
			name:  "base v3 endpoint",
			input: "https://us.api.konghq.com/v3",
			want:  "https://us.api.konghq.com",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeOpenMeterURL(tc.input)
			if got != tc.want {
				t.Fatalf("normalizeOpenMeterURL(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestBuildCustomerKeyDeterministic(t *testing.T) {
	first, err := buildCustomerKey("tenant_1", "app_123", "user_abc")
	if err != nil {
		t.Fatalf("buildCustomerKey first failed: %v", err)
	}
	second, err := buildCustomerKey(" tenant_1 ", " app_123 ", " user_abc ")
	if err != nil {
		t.Fatalf("buildCustomerKey second failed: %v", err)
	}
	if first != second {
		t.Fatalf("customer key must be deterministic: %q != %q", first, second)
	}
}
