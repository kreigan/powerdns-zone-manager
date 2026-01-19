package manager

import (
	"context"
	"errors"
	"testing"

	"github.com/kreigan/powerdns-zone-manager/internal/config"
	"github.com/kreigan/powerdns-zone-manager/internal/logger"
	"github.com/kreigan/powerdns-zone-manager/internal/powerdns"
)

// testLogger returns a quiet logger for tests
func testLogger() *logger.Logger {
	return logger.New(false)
}

// MockClient implements PowerDNSClient for testing
type MockClient struct {
	zones         map[string]*powerdns.Zone
	createZoneErr error
	getZoneErr    error
	patchZoneErr  error
	patchCalls    []powerdns.ZonePatch
}

func NewMockClient() *MockClient {
	return &MockClient{
		zones:      make(map[string]*powerdns.Zone),
		patchCalls: []powerdns.ZonePatch{},
	}
}

func (m *MockClient) CreateZone(_ context.Context, zone *powerdns.Zone) (*powerdns.Zone, error) {
	if m.createZoneErr != nil {
		return nil, m.createZoneErr
	}
	created := *zone
	m.zones[zone.Name] = &created
	return &created, nil
}

func (m *MockClient) GetZone(_ context.Context, zoneID string) (*powerdns.Zone, error) {
	if m.getZoneErr != nil {
		return nil, m.getZoneErr
	}
	if zone, ok := m.zones[zoneID]; ok {
		return zone, nil
	}
	return nil, nil // Zone not found
}

func (m *MockClient) PatchZone(_ context.Context, _ string, patch *powerdns.ZonePatch) error {
	if m.patchZoneErr != nil {
		return m.patchZoneErr
	}
	m.patchCalls = append(m.patchCalls, *patch)
	return nil
}

func TestManager_Apply_CreateZone(t *testing.T) {
	client := NewMockClient()
	mgr := NewManager(client, "zone-manager", testLogger())

	cfg := &config.Config{
		Zones: map[string]config.Zone{
			"example.com": {
				Kind:        "Native",
				Nameservers: []string{"ns1.example.com."},
				RRsets: []config.RRsetInput{
					{
						Name:    "www",
						Type:    "A",
						Records: "192.168.1.1",
					},
				},
			},
		},
	}

	result, err := mgr.Apply(context.Background(), cfg, ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if result.ZonesCreated != 1 {
		t.Errorf("Expected 1 zone created, got %d", result.ZonesCreated)
	}

	// Should create NS + www A record
	if result.RRsetsCreated != 2 {
		t.Errorf("Expected 2 rrsets created (NS + A), got %d", result.RRsetsCreated)
	}

	// Check zone was created with correct account
	zone, ok := client.zones["example.com."]
	if !ok {
		t.Fatal("Zone was not created")
	}
	if zone.Account != "zone-manager" {
		t.Errorf("Expected account 'zone-manager', got %s", zone.Account)
	}
}

func TestManager_Apply_DryRun(t *testing.T) {
	client := NewMockClient()
	mgr := NewManager(client, "zone-manager", testLogger())

	cfg := &config.Config{
		Zones: map[string]config.Zone{
			"example.com": {
				Nameservers: []string{"ns1.example.com."},
				RRsets: []config.RRsetInput{
					{
						Name:    "www",
						Type:    "A",
						Records: "192.168.1.1",
					},
				},
			},
		},
	}

	result, err := mgr.Apply(context.Background(), cfg, ApplyOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if result.ZonesCreated != 1 {
		t.Errorf("Expected 1 zone in dry run, got %d", result.ZonesCreated)
	}

	// No actual zones should be created
	if len(client.zones) != 0 {
		t.Error("Expected no zones created in dry run")
	}

	// No patches should be made
	if len(client.patchCalls) != 0 {
		t.Error("Expected no patches in dry run")
	}
}

func TestManager_Apply_ExistingManagedZone(t *testing.T) {
	client := NewMockClient()
	// Pre-populate with existing managed zone
	client.zones["example.com."] = &powerdns.Zone{
		Name:    "example.com.",
		Account: "zone-manager",
		RRsets: []powerdns.RRset{
			{
				Name: "example.com.",
				Type: "NS",
				TTL:  300,
				Records: []powerdns.Record{
					{Content: "ns1.example.com.", Disabled: false},
				},
				Comments: []powerdns.Comment{
					{Content: "Managed", Account: "zone-manager"},
				},
			},
			{
				Name: "old.example.com.",
				Type: "A",
				TTL:  300,
				Records: []powerdns.Record{
					{Content: "192.168.1.99", Disabled: false},
				},
				Comments: []powerdns.Comment{
					{Content: "Managed", Account: "zone-manager"},
				},
			},
		},
	}

	mgr := NewManager(client, "zone-manager", testLogger())

	cfg := &config.Config{
		Zones: map[string]config.Zone{
			"example.com": {
				Nameservers: []string{"ns1.example.com."},
				RRsets: []config.RRsetInput{
					{
						Name:    "www",
						Type:    "A",
						Records: "192.168.1.1",
					},
				},
			},
		},
	}

	result, err := mgr.Apply(context.Background(), cfg, ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if result.ZonesCreated != 0 {
		t.Errorf("Expected 0 zones created, got %d", result.ZonesCreated)
	}

	// www should be created, old should be deleted
	if result.RRsetsCreated != 1 {
		t.Errorf("Expected 1 rrset created, got %d", result.RRsetsCreated)
	}
	if result.RRsetsDeleted != 1 {
		t.Errorf("Expected 1 rrset deleted (orphaned), got %d", result.RRsetsDeleted)
	}
}

func TestManager_Apply_NonManagedRecordNotTouched(t *testing.T) {
	client := NewMockClient()
	// Pre-populate with existing managed zone but non-managed record
	client.zones["example.com."] = &powerdns.Zone{
		Name:    "example.com.",
		Account: "zone-manager",
		RRsets: []powerdns.RRset{
			{
				Name: "manual.example.com.",
				Type: "A",
				TTL:  300,
				Records: []powerdns.Record{
					{Content: "192.168.1.99", Disabled: false},
				},
				Comments: []powerdns.Comment{
					{Content: "Manual record", Account: "other-account"},
				},
			},
		},
	}

	mgr := NewManager(client, "zone-manager", testLogger())

	cfg := &config.Config{
		Zones: map[string]config.Zone{
			"example.com": {
				RRsets: []config.RRsetInput{
					{
						Name:    "www",
						Type:    "A",
						Records: "192.168.1.1",
					},
				},
			},
		},
	}

	result, err := mgr.Apply(context.Background(), cfg, ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	// Only www should be created, manual record should NOT be deleted
	if result.RRsetsCreated != 1 {
		t.Errorf("Expected 1 rrset created, got %d", result.RRsetsCreated)
	}
	if result.RRsetsDeleted != 0 {
		t.Errorf("Expected 0 rrsets deleted (non-managed should not be touched), got %d", result.RRsetsDeleted)
	}
}

func TestManager_Apply_UpdateManagedRecord(t *testing.T) {
	client := NewMockClient()
	client.zones["example.com."] = &powerdns.Zone{
		Name:    "example.com.",
		Account: "zone-manager",
		RRsets: []powerdns.RRset{
			{
				Name: "www.example.com.",
				Type: "A",
				TTL:  300,
				Records: []powerdns.Record{
					{Content: "192.168.1.1", Disabled: false},
				},
				Comments: []powerdns.Comment{
					{Content: "Managed", Account: "zone-manager"},
				},
			},
		},
	}

	mgr := NewManager(client, "zone-manager", testLogger())

	cfg := &config.Config{
		Zones: map[string]config.Zone{
			"example.com": {
				RRsets: []config.RRsetInput{
					{
						Name:    "www",
						Type:    "A",
						Records: "192.168.1.2", // Changed IP
					},
				},
			},
		},
	}

	result, err := mgr.Apply(context.Background(), cfg, ApplyOptions{})
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if result.RRsetsUpdated != 1 {
		t.Errorf("Expected 1 rrset updated, got %d", result.RRsetsUpdated)
	}
}

func TestManager_Apply_UnmanagedZoneAllowsRRsets(t *testing.T) {
	client := NewMockClient()
	// Zone exists but with different account
	client.zones["example.com."] = &powerdns.Zone{
		Name:    "example.com.",
		Account: "other-owner",
		RRsets:  []powerdns.RRset{},
	}

	mgr := NewManager(client, "zone-manager", testLogger())

	// Only RRsets, no nameservers - should succeed
	cfg := &config.Config{
		Zones: map[string]config.Zone{
			"example.com": {
				RRsets: []config.RRsetInput{
					{
						Name:    "www",
						Type:    "A",
						Records: "192.168.1.1",
					},
				},
			},
		},
	}

	result, err := mgr.Apply(context.Background(), cfg, ApplyOptions{})
	if err != nil {
		t.Fatalf("Expected success for RRsets on unmanaged zone, got error: %v", err)
	}
	if result.RRsetsCreated != 1 {
		t.Errorf("Expected 1 RRset created, got %d", result.RRsetsCreated)
	}
}

func TestManager_Apply_UnmanagedZoneSkipsNameservers(t *testing.T) {
	client := NewMockClient()
	// Zone exists but with different account
	client.zones["example.com."] = &powerdns.Zone{
		Name:    "example.com.",
		Account: "other-owner",
		RRsets:  []powerdns.RRset{},
	}

	mgr := NewManager(client, "zone-manager", testLogger())

	// Nameservers specified on unmanaged zone - should be silently skipped
	cfg := &config.Config{
		Zones: map[string]config.Zone{
			"example.com": {
				Nameservers: []string{"ns1.example.com."},
				RRsets: []config.RRsetInput{
					{
						Name:    "www",
						Type:    "A",
						Records: "192.168.1.1",
					},
				},
			},
		},
	}

	result, err := mgr.Apply(context.Background(), cfg, ApplyOptions{})
	if err != nil {
		t.Fatalf("Expected success (nameservers silently skipped), got error: %v", err)
	}
	// Only RRset created, NS should be skipped
	if result.RRsetsCreated != 1 {
		t.Errorf("Expected 1 RRset created (NS skipped), got %d", result.RRsetsCreated)
	}
}

func TestManager_Apply_ClientError(t *testing.T) {
	client := NewMockClient()
	client.getZoneErr = errors.New("connection refused")

	mgr := NewManager(client, "zone-manager", testLogger())

	cfg := &config.Config{
		Zones: map[string]config.Zone{
			"example.com": {
				Nameservers: []string{"ns1.example.com."},
			},
		},
	}

	_, err := mgr.Apply(context.Background(), cfg, ApplyOptions{})
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
}

func TestBuildFQDN(t *testing.T) {
	mgr := &Manager{}

	tests := []struct {
		name     string
		zoneID   string
		expected string
	}{
		{"www", "example.com.", "www.example.com."},
		{"@", "example.com.", "example.com."},
		{"www.example.com.", "example.com.", "www.example.com."}, // Already FQDN
		{"sub.domain", "example.com.", "sub.domain.example.com."},
	}

	for _, tt := range tests {
		result := mgr.buildFQDN(tt.name, tt.zoneID)
		if result != tt.expected {
			t.Errorf("buildFQDN(%s, %s) = %s, want %s", tt.name, tt.zoneID, result, tt.expected)
		}
	}
}

func TestIsManaged(t *testing.T) {
	mgr := &Manager{accountName: "zone-manager"}

	tests := []struct {
		name     string
		rrset    powerdns.RRset
		expected bool
	}{
		{
			name: "managed",
			rrset: powerdns.RRset{
				Comments: []powerdns.Comment{
					{Account: "zone-manager"},
				},
			},
			expected: true,
		},
		{
			name: "not managed - different account",
			rrset: powerdns.RRset{
				Comments: []powerdns.Comment{
					{Account: "other-account"},
				},
			},
			expected: false,
		},
		{
			name: "not managed - no comments",
			rrset: powerdns.RRset{
				Comments: []powerdns.Comment{},
			},
			expected: false,
		},
		{
			name: "managed - one of many comments",
			rrset: powerdns.RRset{
				Comments: []powerdns.Comment{
					{Account: "other"},
					{Account: "zone-manager"},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mgr.isManaged(tt.rrset)
			if result != tt.expected {
				t.Errorf("isManaged() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestShouldUpdateRRset(t *testing.T) {
	mgr := &Manager{}

	tests := []struct {
		name     string
		desired  powerdns.RRset
		existing powerdns.RRset
		expected bool
	}{
		{
			name: "no change",
			desired: powerdns.RRset{
				TTL: 300,
				Records: []powerdns.Record{
					{Content: "192.168.1.1", Disabled: false},
				},
			},
			existing: powerdns.RRset{
				TTL: 300,
				Records: []powerdns.Record{
					{Content: "192.168.1.1", Disabled: false},
				},
			},
			expected: false,
		},
		{
			name: "TTL changed",
			desired: powerdns.RRset{
				TTL: 600,
				Records: []powerdns.Record{
					{Content: "192.168.1.1", Disabled: false},
				},
			},
			existing: powerdns.RRset{
				TTL: 300,
				Records: []powerdns.Record{
					{Content: "192.168.1.1", Disabled: false},
				},
			},
			expected: true,
		},
		{
			name: "content changed",
			desired: powerdns.RRset{
				TTL: 300,
				Records: []powerdns.Record{
					{Content: "192.168.1.2", Disabled: false},
				},
			},
			existing: powerdns.RRset{
				TTL: 300,
				Records: []powerdns.Record{
					{Content: "192.168.1.1", Disabled: false},
				},
			},
			expected: true,
		},
		{
			name: "disabled changed",
			desired: powerdns.RRset{
				TTL: 300,
				Records: []powerdns.Record{
					{Content: "192.168.1.1", Disabled: true},
				},
			},
			existing: powerdns.RRset{
				TTL: 300,
				Records: []powerdns.Record{
					{Content: "192.168.1.1", Disabled: false},
				},
			},
			expected: true,
		},
		{
			name: "record count changed",
			desired: powerdns.RRset{
				TTL: 300,
				Records: []powerdns.Record{
					{Content: "192.168.1.1", Disabled: false},
					{Content: "192.168.1.2", Disabled: false},
				},
			},
			existing: powerdns.RRset{
				TTL: 300,
				Records: []powerdns.Record{
					{Content: "192.168.1.1", Disabled: false},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mgr.shouldUpdateRRset(tt.desired, tt.existing)
			if result != tt.expected {
				t.Errorf("shouldUpdateRRset() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestNormalizeNameservers(t *testing.T) {
	mgr := &Manager{}

	tests := []struct {
		ns       string
		zoneID   string
		expected string
	}{
		{"ns1.example.com.", "example.com.", "ns1.example.com."},
		{"ns1", "example.com.", "ns1.example.com."},
		{"ns.other.com.", "example.com.", "ns.other.com."},
	}

	for _, tt := range tests {
		result := mgr.normalizeNameserver(tt.ns, tt.zoneID)
		if result != tt.expected {
			t.Errorf("normalizeNameserver(%s, %s) = %s, want %s", tt.ns, tt.zoneID, result, tt.expected)
		}
	}
}

func TestRRsetKey(t *testing.T) {
	tests := []struct {
		name       string
		recordType string
		expected   string
	}{
		{"example.com.", "A", "example.com./A"},
		{"Example.COM.", "a", "example.com./A"},
		{"www.example.com.", "AAAA", "www.example.com./AAAA"},
	}

	for _, tt := range tests {
		result := rrsetKey(tt.name, tt.recordType)
		if result != tt.expected {
			t.Errorf("rrsetKey(%s, %s) = %s, want %s", tt.name, tt.recordType, result, tt.expected)
		}
	}
}
