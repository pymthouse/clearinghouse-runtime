package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadEnvFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte("OPENMETER_API_KEY=kpat_from_file\n"), 0644)

	t.Setenv("OPENMETER_API_KEY", "kpat_existing")
	if err := LoadEnvFile(envPath, true); err != nil {
		t.Fatalf("LoadEnvFile: %v", err)
	}
	if got := os.Getenv("OPENMETER_API_KEY"); got != "kpat_existing" {
		t.Errorf("should not override existing env, got %q", got)
	}

	os.Unsetenv("OPENMETER_API_KEY")
	if err := LoadEnvFile(envPath, true); err != nil {
		t.Fatalf("LoadEnvFile: %v", err)
	}
	if got := os.Getenv("OPENMETER_API_KEY"); got != "kpat_from_file" {
		t.Errorf("got %q, want kpat_from_file", got)
	}
}

func TestLoadEnvFileMissingDefault(t *testing.T) {
	if err := LoadEnvFile(filepath.Join(t.TempDir(), "missing.env"), false); err != nil {
		t.Fatalf("optional missing file should not error: %v", err)
	}
}

func TestLoadEnvFileMissingExplicit(t *testing.T) {
	err := LoadEnvFile(filepath.Join(t.TempDir(), "missing.env"), true)
	if err == nil {
		t.Fatal("expected error for explicit missing env file")
	}
}

func TestPreprocessArgsHelp(t *testing.T) {
	_, _, _, help, err := PreprocessArgs([]string{"--help"})
	if err != nil || help != HelpBrief {
		t.Fatalf("help=%v err=%v", help, err)
	}
	_, _, _, help, err = PreprocessArgs([]string{"--help-all"})
	if err != nil || help != HelpAll {
		t.Fatalf("help=%v err=%v", help, err)
	}
}

func TestPreprocessArgsEnvFile(t *testing.T) {
	remaining, path, explicit, help, err := PreprocessArgs([]string{"--env-file", "custom.env", "--skip-auth0"})
	if err != nil || help != HelpNone {
		t.Fatalf("err=%v help=%v", err, help)
	}
	if path != "custom.env" || !explicit {
		t.Fatalf("path=%q explicit=%v", path, explicit)
	}
	if len(remaining) != 1 || remaining[0] != "--skip-auth0" {
		t.Fatalf("remaining=%v", remaining)
	}
}

func TestParseFromEnvFile(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	os.WriteFile(envPath, []byte(`AUTH0_DOMAIN=test.auth0.com
AUTH0_MGMT_CLIENT_ID=cid
AUTH0_MGMT_CLIENT_SECRET=csec
OPENMETER_API_KEY=kpat_test
`), 0644)

	for _, k := range []string{"AUTH0_DOMAIN", "AUTH0_MGMT_CLIENT_ID", "AUTH0_MGMT_CLIENT_SECRET", "OPENMETER_API_KEY"} {
		os.Unsetenv(k)
	}
	if err := LoadEnvFile(envPath, true); err != nil {
		t.Fatalf("LoadEnvFile: %v", err)
	}

	cfg, err := parseForTest()
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Auth0Domain != "test.auth0.com" {
		t.Errorf("Auth0Domain = %s", cfg.Auth0Domain)
	}
	if cfg.OpenmeterAPIKey != "kpat_test" {
		t.Errorf("OpenmeterAPIKey = %s", cfg.OpenmeterAPIKey)
	}
}
