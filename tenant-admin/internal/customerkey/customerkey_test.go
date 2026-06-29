package customerkey

import "testing"

func TestBuildMatchesBuilderSdkFormat(t *testing.T) {
	key, err := Build("app_acme", "user_1")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	// Must be exactly clientId:externalUserId — same as builder-sdk/pymthouse.
	if key != "app_acme:user_1" {
		t.Fatalf("unexpected key %q", key)
	}
}

func TestBuildRejectsEmptySegments(t *testing.T) {
	if _, err := Build("", "user_1"); err == nil {
		t.Fatalf("expected error for empty clientId")
	}
	if _, err := Build("app_acme", "  "); err == nil {
		t.Fatalf("expected error for blank externalUserId")
	}
}

func TestParseFirstColon(t *testing.T) {
	clientID, externalUserID, ok := Parse("app_acme:user:with:colons")
	if !ok {
		t.Fatalf("Parse failed")
	}
	if clientID != "app_acme" || externalUserID != "user:with:colons" {
		t.Fatalf("unexpected parse: %q %q", clientID, externalUserID)
	}
	if _, _, ok := Parse("nocolon"); ok {
		t.Fatalf("expected parse to fail for key without colon")
	}
}
