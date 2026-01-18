package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFromFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	configContent := `zones:
  example.com:
    nameservers:
      - ns1.example.com.
    rrsets:
      - name: www
        type: A
        ttl: 300
        records:
          - content: 192.168.1.1
            disabled: false
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	if len(cfg.Zones) != 1 {
		t.Errorf("Expected 1 zone, got %d", len(cfg.Zones))
	}

	zone, ok := cfg.Zones["example.com"]
	if !ok {
		t.Fatal("Zone example.com not found")
	}

	if len(zone.RRsets) != 1 {
		t.Errorf("Expected 1 rrset, got %d", len(zone.RRsets))
	}
}

func TestLoadFromFile_RecordFormats(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Test various record formats as in zones-example.yml
	configContent := `zones:
  example.local:
    nameservers:
      - ns.example.local

    rrsets:
      - name: ns
        type: A
        ttl: 86400
        records:
          - content: 10.1.21.3
            comment: Record comment

      - name: '@'
        type: A
        records: 192.168.2.1

      - name: www
        type: A
        records:
          - 192.168.2.1

      - name: test-auto
        type: TXT
        records:
          - Test TXT record
          - content: Another TXT record
            disabled: true
`

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	cfg, err := LoadFromFile(configPath)
	if err != nil {
		t.Fatalf("LoadFromFile failed: %v", err)
	}

	zone, ok := cfg.Zones["example.local"]
	if !ok {
		t.Fatal("Zone example.local not found")
	}

	if len(zone.RRsets) != 4 {
		t.Errorf("Expected 4 rrsets, got %d", len(zone.RRsets))
	}

	// Test NormalizeRRsets
	rrsets, err := zone.NormalizeRRsets()
	if err != nil {
		t.Fatalf("NormalizeRRsets failed: %v", err)
	}

	// Check ns record
	if len(rrsets[0].Records) != 1 || rrsets[0].Records[0].Content != "10.1.21.3" {
		t.Errorf("ns record not parsed correctly: %+v", rrsets[0])
	}
	if rrsets[0].TTL != 86400 {
		t.Errorf("ns TTL expected 86400, got %d", rrsets[0].TTL)
	}

	// Check @ record (single string)
	if len(rrsets[1].Records) != 1 || rrsets[1].Records[0].Content != "192.168.2.1" {
		t.Errorf("@ record not parsed correctly: %+v", rrsets[1])
	}
	if rrsets[1].TTL != 300 { // Default TTL
		t.Errorf("@ TTL expected 300 (default), got %d", rrsets[1].TTL)
	}

	// Check www record (list of strings)
	if len(rrsets[2].Records) != 1 || rrsets[2].Records[0].Content != "192.168.2.1" {
		t.Errorf("www record not parsed correctly: %+v", rrsets[2])
	}

	// Check test-auto record (mixed list)
	if len(rrsets[3].Records) != 2 {
		t.Errorf("test-auto expected 2 records, got %d", len(rrsets[3].Records))
	}
	if rrsets[3].Records[0].Content != "Test TXT record" {
		t.Errorf("test-auto[0] content wrong: %s", rrsets[3].Records[0].Content)
	}
	if rrsets[3].Records[0].Disabled != false {
		t.Error("test-auto[0] should not be disabled")
	}
	if rrsets[3].Records[1].Content != "Another TXT record" {
		t.Errorf("test-auto[1] content wrong: %s", rrsets[3].Records[1].Content)
	}
	if rrsets[3].Records[1].Disabled != true {
		t.Error("test-auto[1] should be disabled")
	}
}

func TestValidate_NameserversRequired(t *testing.T) {
	cfg := &Config{
		Zones: map[string]Zone{
			"example.com": {
				RRsets: []RRsetInput{
					{Name: "www", Type: "A", Records: "192.168.1.1"},
				},
			},
		},
	}

	// Zone doesn't exist
	existingZones := map[string]ZoneState{}

	err := cfg.Validate(existingZones)
	if err == nil {
		t.Error("Expected validation error for missing nameservers, got nil")
	}
	if !strings.Contains(err.Error(), "nameservers are required") {
		t.Errorf("Expected nameservers error, got: %v", err)
	}
}

func TestValidate_NameserversNotRequiredForExisting(t *testing.T) {
	cfg := &Config{
		Zones: map[string]Zone{
			"example.com": {
				RRsets: []RRsetInput{
					{Name: "www", Type: "A", Records: "192.168.1.1"},
				},
			},
		},
	}

	// Zone exists and is managed
	existingZones := map[string]ZoneState{
		"example.com.": {Exists: true, IsManaged: true},
	}

	err := cfg.Validate(existingZones)
	if err != nil {
		t.Errorf("Expected no error for existing zone, got: %v", err)
	}
}

func TestValidate_UnmanagedZoneFails(t *testing.T) {
	cfg := &Config{
		Zones: map[string]Zone{
			"example.com": {
				RRsets: []RRsetInput{
					{Name: "www", Type: "A", Records: "192.168.1.1"},
				},
			},
		},
	}

	// Zone exists but is NOT managed
	existingZones := map[string]ZoneState{
		"example.com.": {Exists: true, IsManaged: false},
	}

	err := cfg.Validate(existingZones)
	if err == nil {
		t.Error("Expected validation error for unmanaged zone, got nil")
	}
	if !strings.Contains(err.Error(), "not managed") {
		t.Errorf("Expected 'not managed' error, got: %v", err)
	}
}

func TestValidate_ProhibitedNSType(t *testing.T) {
	cfg := &Config{
		Zones: map[string]Zone{
			"example.com": {
				Nameservers: []string{"ns1.example.com."},
				RRsets: []RRsetInput{
					{
						Name:    "@",
						Type:    "NS",
						Records: "ns1.example.com.",
					},
				},
			},
		},
	}

	existingZones := map[string]ZoneState{}

	err := cfg.Validate(existingZones)
	if err == nil {
		t.Error("Expected validation error for NS record in rrsets, got nil")
	}
	if !strings.Contains(err.Error(), "NS records must be managed via 'nameservers' property") {
		t.Errorf("Expected NS property error, got: %v", err)
	}
}

func TestValidate_ProhibitedSOAType(t *testing.T) {
	cfg := &Config{
		Zones: map[string]Zone{
			"example.com": {
				Nameservers: []string{"ns1.example.com."},
				RRsets: []RRsetInput{
					{
						Name:    "@",
						Type:    "SOA",
						Records: "ns1.example.com. hostmaster.example.com. 1 7200 900 1209600 86400",
					},
				},
			},
		},
	}

	existingZones := map[string]ZoneState{}

	err := cfg.Validate(existingZones)
	if err == nil {
		t.Error("Expected validation error for SOA record, got nil")
	}
	if !strings.Contains(err.Error(), "SOA records are managed by PowerDNS") {
		t.Errorf("Expected SOA error, got: %v", err)
	}
}

func TestValidate_DuplicateRRsets(t *testing.T) {
	cfg := &Config{
		Zones: map[string]Zone{
			"example.com": {
				Nameservers: []string{"ns1.example.com."},
				RRsets: []RRsetInput{
					{Name: "www", Type: "A", Records: "192.168.1.1"},
					{Name: "www", Type: "A", Records: "192.168.1.2"},
				},
			},
		},
	}

	existingZones := map[string]ZoneState{}

	err := cfg.Validate(existingZones)
	if err == nil {
		t.Error("Expected validation error for duplicate RRsets, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate RRset") {
		t.Errorf("Expected duplicate error, got: %v", err)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	cfg := &Config{
		Zones: map[string]Zone{
			"example.com": {
				// Missing nameservers
				RRsets: []RRsetInput{
					{Name: "", Type: "A", Records: "192.168.1.1"},   // Missing name
					{Name: "www", Type: "", Records: "192.168.1.1"}, // Missing type
					{Name: "test", Type: "A", Records: nil},         // Missing records
					{Name: "@", Type: "NS", Records: "ns.example."}, // Prohibited type
				},
			},
		},
	}

	existingZones := map[string]ZoneState{}

	validationErr := cfg.Validate(existingZones)
	if validationErr == nil {
		t.Error("Expected validation errors, got nil")
	}

	// Should have multiple errors
	if len(validationErr.Errors) < 4 {
		t.Errorf("Expected at least 4 errors, got %d: %v", len(validationErr.Errors), validationErr.Errors)
	}
}

func TestValidate_InvalidKind(t *testing.T) {
	cfg := &Config{
		Zones: map[string]Zone{
			"example.com": {
				Kind:        "InvalidKind",
				Nameservers: []string{"ns1.example.com."},
			},
		},
	}

	existingZones := map[string]ZoneState{}

	err := cfg.Validate(existingZones)
	if err == nil {
		t.Error("Expected validation error for invalid kind, got nil")
	}
	if !strings.Contains(err.Error(), "invalid kind") {
		t.Errorf("Expected kind error, got: %v", err)
	}
}

func TestNormalizeZone_Defaults(t *testing.T) {
	zone := &Zone{
		Nameservers: []string{"ns1.example.com."},
	}

	zone.NormalizeZone()

	if zone.Kind != "Native" {
		t.Errorf("Expected kind Native, got %s", zone.Kind)
	}
}

func TestNormalizeRRsets_DefaultTTL(t *testing.T) {
	ttl := uint32(600)
	zone := &Zone{
		RRsets: []RRsetInput{
			{Name: "www", Type: "A", Records: "192.168.1.1"},             // No TTL
			{Name: "mail", Type: "A", TTL: &ttl, Records: "192.168.1.2"}, // With TTL
		},
	}

	rrsets, err := zone.NormalizeRRsets()
	if err != nil {
		t.Fatalf("NormalizeRRsets failed: %v", err)
	}

	if rrsets[0].TTL != 300 {
		t.Errorf("Expected default TTL 300, got %d", rrsets[0].TTL)
	}

	if rrsets[1].TTL != 600 {
		t.Errorf("Expected TTL 600, got %d", rrsets[1].TTL)
	}
}

func TestCanonicalZoneName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"example.com", "example.com."},
		{"example.com.", "example.com."},
		{"sub.example.com", "sub.example.com."},
	}

	for _, tt := range tests {
		result := CanonicalZoneName(tt.input)
		if result != tt.expected {
			t.Errorf("CanonicalZoneName(%s) = %s, want %s", tt.input, result, tt.expected)
		}
	}
}
