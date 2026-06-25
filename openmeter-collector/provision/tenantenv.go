package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func tenantEnvPath(dataDir string, tenantID string) string {
	return filepath.Join(strings.TrimSpace(dataDir), ".env."+strings.TrimSpace(tenantID))
}

func loadTenantOpenMeterAPIKey(dataDir string, tenantID string) (string, error) {
	path := tenantEnvPath(dataDir, tenantID)
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "OPENMETER_API_KEY=") {
			return strings.TrimSpace(strings.TrimPrefix(trimmedLine, "OPENMETER_API_KEY=")), nil
		}
	}
	return "", fmt.Errorf("OPENMETER_API_KEY missing in %s", path)
}
