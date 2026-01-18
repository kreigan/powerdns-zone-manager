package config

import (
	"strings"
	"testing"
)

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

	existingZones := map[string]ZoneState{}

	err := cfg.Validate(existingZones)
	if err == nil {
		t.Error("Expected validation error for missing nameservers, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "nameservers are required") {
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

	existingZones := map[string]ZoneState{
		"example.com.": {Exists: true, IsManaged: false},
	}

	err := cfg.Validate(existingZones)
	if err == nil {
		t.Error("Expected validation error for unmanaged zone, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "not managed") {
		t.Errorf("Expected not managed error, got: %v", err)
	}
}

func TestValidate_ProhibitedNS(t *testing.T) {
	cfg := &Config{
		Zones: map[string]Zone{
			"example.com": {
				Nameservers: []string{"ns1.example.com."},
				RRsets: []RRsetInput{
					{Name: "@", Type: "NS", Records: "ns1.example.com."},
				},
			},
		},
	}

	existingZones := map[string]ZoneState{}

	err := cfg.Validate(existingZones)
	if err == nil {
		t.Error("Expected validation error for NS record in rrsets, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "NS records must be managed via") {
		t.Errorf("Expected NS property error, got: %v", err)
	}
}

func TestValidate_ProhibitedSOA(t *testing.T) {
	cfg := &Config{
		Zones: map[string]Zone{
			"example.com": {
				Nameservers: []string{"ns1.example.com."},
				RRsets: []RRsetInput{
					{Name: "@", Type: "SOA", Records: "ns1 hostmaster 1 7200 900 1209600 86400"},
				},
			},
		},
	}

	existingZones := map[string]ZoneState{}

	err := cfg.Validate(existingZones)
	if err == nil {
		t.Error("Expected validation error for SOA record, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "SOA records are managed by PowerDNS") {
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
	if err != nil && !strings.Contains(err.Error(), "duplicate RRset") {
		t.Errorf("Expected duplicate error, got: %v", err)
	}
}

func TestValidate_MultipleErrors(t *testing.T) {
	cfg := &Config{
		Zones: map[string]Zone{
			"example.com": {
				RRsets: []RRsetInput{
					{Name: "", Type: "A", Records: "192.168.1.1"},
					{Name: "www", Type: "", Records: "192.168.1.1"},
					{Name: "test", Type: "A", Records: nil},
					{Name: "@", Type: "NS", Records: "ns.example."},
				},
			},
		},
	}

	existingZones := map[string]ZoneState{}

	validationErr := cfg.Validate(existingZones)
	requireValidationErr(t, validationErr, 4)
}

func requireValidationErr(t *testing.T, validationErr *ValidationError, minErrors int) {
	t.Helper()
	if validationErr == nil {
		t.Fatal("Expected validation errors, got nil")
	}
	if len(validationErr.Errors) < minErrors {
		t.Errorf("Expected at least %d errors, got %d: %v",
			minErrors, len(validationErr.Errors), validationErr.Errors)
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
	if err != nil && !strings.Contains(err.Error(), "invalid kind") {
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
			{Name: "www", Type: "A", Records: "192.168.1.1"},
			{Name: "mail", Type: "A", TTL: &ttl, Records: "192.168.1.2"},
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
