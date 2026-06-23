package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type TenantRecord struct {
	TenantID        string    `json:"tenantId"`
	TenantName      string    `json:"tenantName"`
	Auth0OrgID      string    `json:"auth0OrgId"`
	KonnectTeamID   string    `json:"konnectTeamId"`
	SystemAccountID string    `json:"systemAccountId"`
	SystemTokenID   string    `json:"systemTokenId"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

type TenantAppRecord struct {
	TenantID  string    `json:"tenantId"`
	ClientID  string    `json:"clientId"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type FileRegistry struct {
	tenantsPath string
	appsPath    string
	dataDir     string
}

func NewFileRegistry(dataDir string) *FileRegistry {
	trimmedDataDir := strings.TrimSpace(dataDir)
	return &FileRegistry{
		tenantsPath: filepath.Join(trimmedDataDir, "tenants.json"),
		appsPath:    filepath.Join(trimmedDataDir, "apps.json"),
		dataDir:     trimmedDataDir,
	}
}

func (r *FileRegistry) Upsert(record TenantRecord) error {
	if err := os.MkdirAll(filepath.Dir(r.tenantsPath), 0o755); err != nil {
		return fmt.Errorf("create registry dir: %w", err)
	}
	records, err := r.loadTenants()
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	record.UpdatedAt = now
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}

	replaced := false
	for i := range records {
		if strings.EqualFold(records[i].TenantID, record.TenantID) {
			record.CreatedAt = records[i].CreatedAt
			records[i] = record
			replaced = true
			break
		}
	}
	if !replaced {
		records = append(records, record)
	}

	return r.saveTenants(records)
}

func (r *FileRegistry) UpsertApp(record TenantAppRecord) error {
	if err := os.MkdirAll(filepath.Dir(r.appsPath), 0o755); err != nil {
		return fmt.Errorf("create app registry dir: %w", err)
	}
	records, err := r.loadApps()
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	record.UpdatedAt = now
	if record.CreatedAt.IsZero() {
		record.CreatedAt = now
	}

	replaced := false
	for i := range records {
		if strings.EqualFold(records[i].TenantID, record.TenantID) && strings.EqualFold(records[i].ClientID, record.ClientID) {
			record.CreatedAt = records[i].CreatedAt
			records[i] = record
			replaced = true
			break
		}
	}
	if !replaced {
		records = append(records, record)
	}

	return r.saveApps(records)
}

func (r *FileRegistry) Get(tenantID string) (*TenantRecord, error) {
	records, err := r.loadTenants()
	if err != nil {
		return nil, err
	}
	for i := range records {
		if strings.EqualFold(records[i].TenantID, strings.TrimSpace(tenantID)) {
			copyRecord := records[i]
			return &copyRecord, nil
		}
	}
	return nil, nil
}

func (r *FileRegistry) List() ([]TenantRecord, error) {
	records, err := r.loadTenants()
	if err != nil {
		return nil, err
	}
	sort.Slice(records, func(i, j int) bool {
		return strings.ToLower(records[i].TenantID) < strings.ToLower(records[j].TenantID)
	})
	return records, nil
}

func (r *FileRegistry) ListApps(tenantID string) ([]TenantAppRecord, error) {
	records, err := r.loadApps()
	if err != nil {
		return nil, err
	}
	filtered := make([]TenantAppRecord, 0, len(records))
	for _, record := range records {
		if strings.EqualFold(record.TenantID, strings.TrimSpace(tenantID)) {
			filtered = append(filtered, record)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return strings.ToLower(filtered[i].ClientID) < strings.ToLower(filtered[j].ClientID)
	})
	return filtered, nil
}

func (r *FileRegistry) GetTenantForClient(clientID string) (*TenantRecord, error) {
	targetClientID := strings.TrimSpace(clientID)
	if targetClientID == "" {
		return nil, nil
	}
	appRecords, err := r.loadApps()
	if err != nil {
		return nil, err
	}
	for _, appRecord := range appRecords {
		if strings.EqualFold(appRecord.ClientID, targetClientID) {
			return r.Get(appRecord.TenantID)
		}
	}
	return nil, nil
}

func (r *FileRegistry) GetSPAT(tenantID string) (string, error) {
	tenantPath := filepath.Join(r.dataDir, ".env."+strings.TrimSpace(tenantID))
	raw, err := os.ReadFile(tenantPath)
	if err != nil {
		return "", fmt.Errorf("read tenant env file: %w", err)
	}
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)
		if strings.HasPrefix(trimmedLine, "OPENMETER_API_KEY=") {
			return strings.TrimSpace(strings.TrimPrefix(trimmedLine, "OPENMETER_API_KEY=")), nil
		}
	}
	return "", fmt.Errorf("OPENMETER_API_KEY missing in tenant env file")
}

func (r *FileRegistry) loadTenants() ([]TenantRecord, error) {
	raw, err := os.ReadFile(r.tenantsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []TenantRecord{}, nil
		}
		return nil, fmt.Errorf("read tenant registry: %w", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return []TenantRecord{}, nil
	}
	var records []TenantRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, fmt.Errorf("decode tenant registry: %w", err)
	}
	return records, nil
}

func (r *FileRegistry) saveTenants(records []TenantRecord) error {
	encoded, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("encode tenant registry: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(r.tenantsPath, encoded, 0o600); err != nil {
		return fmt.Errorf("write tenant registry: %w", err)
	}
	return nil
}

func (r *FileRegistry) loadApps() ([]TenantAppRecord, error) {
	raw, err := os.ReadFile(r.appsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []TenantAppRecord{}, nil
		}
		return nil, fmt.Errorf("read app registry: %w", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return []TenantAppRecord{}, nil
	}
	var records []TenantAppRecord
	if err := json.Unmarshal(raw, &records); err != nil {
		return nil, fmt.Errorf("decode app registry: %w", err)
	}
	return records, nil
}

func (r *FileRegistry) saveApps(records []TenantAppRecord) error {
	encoded, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return fmt.Errorf("encode app registry: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(r.appsPath, encoded, 0o600); err != nil {
		return fmt.Errorf("write app registry: %w", err)
	}
	return nil
}
