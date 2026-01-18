// Package config handles loading and validating zone configuration from YAML files.
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the zone configuration.
type Config struct {
	Zones map[string]Zone `yaml:"zones"`
}

// Zone represents a DNS zone configuration.
type Zone struct {
	Kind        string       `yaml:"kind,omitempty"`
	Nameservers []string     `yaml:"nameservers,omitempty"`
	RRsets      []RRsetInput `yaml:"rrsets,omitempty"`
}

// RRsetInput represents a resource record set as provided in YAML.
type RRsetInput struct {
	Name    string      `yaml:"name"`
	Type    string      `yaml:"type"`
	Records interface{} `yaml:"records"` // Can be string, []string, []RecordInput, or mixed
	TTL     *uint32     `yaml:"ttl,omitempty"`
	Comment string      `yaml:"comment,omitempty"`
}

// RecordInput represents a single DNS record as provided in YAML.
type RecordInput struct {
	Content  string `yaml:"content"`
	Comment  string `yaml:"comment,omitempty"`
	Disabled bool   `yaml:"disabled,omitempty"`
}

// RRset represents a normalized resource record set.
type RRset struct {
	Name    string
	Type    string
	Comment string
	Records []Record
	TTL     uint32
}

// Record represents a normalized single DNS record.
type Record struct {
	Content  string
	Comment  string
	Disabled bool
}

// LoadFromFile loads configuration from a YAML file.
func LoadFromFile(path string) (*Config, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is from CLI argument
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	return &cfg, nil
}

// ValidationError holds all validation errors.
type ValidationError struct {
	Errors []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf(
		"validation failed with %d error(s):\n  - %s",
		len(e.Errors),
		strings.Join(e.Errors, "\n  - "),
	)
}

// Add appends a formatted error message to the validation errors.
func (e *ValidationError) Add(format string, args ...interface{}) {
	e.Errors = append(e.Errors, fmt.Sprintf(format, args...))
}

// HasErrors returns true if there are any validation errors.
func (e *ValidationError) HasErrors() bool {
	return len(e.Errors) > 0
}

// ZoneState holds information about existing zones for validation.
type ZoneState struct {
	Exists    bool
	IsManaged bool
}

// Validate validates the configuration and returns all errors at once.
// existingZones maps canonical zone names to their states.
func (c *Config) Validate(existingZones map[string]ZoneState) *ValidationError {
	errs := &ValidationError{}

	for zoneName, zone := range c.Zones {
		c.validateZone(zoneName, &zone, existingZones, errs)
	}

	if errs.HasErrors() {
		return errs
	}
	return nil
}

func (c *Config) validateZone(
	zoneName string,
	zone *Zone,
	existingZones map[string]ZoneState,
	errs *ValidationError,
) {
	canonicalName := CanonicalZoneName(zoneName)
	state := existingZones[canonicalName]

	// Nameservers is mandatory only if zone is absent
	if !state.Exists && len(zone.Nameservers) == 0 {
		errs.Add("zone %q: nameservers are required when creating a new zone", zoneName)
	}

	// If zone exists but is not managed, we cannot modify it
	if state.Exists && !state.IsManaged {
		errs.Add("zone %q: zone exists but is not managed (account does not match)", zoneName)
	}

	// Validate nameservers format
	for i, ns := range zone.Nameservers {
		if ns == "" {
			errs.Add("zone %q: nameserver[%d] cannot be empty", zoneName, i)
		}
	}

	// Validate kind
	if zone.Kind != "" {
		validKinds := []string{"Native", "Master", "Slave", "Producer", "Consumer"}
		isValid := false
		for _, k := range validKinds {
			if zone.Kind == k {
				isValid = true
				break
			}
		}
		if !isValid {
			errs.Add(
				"zone %q: invalid kind %q, must be one of: Native, Master, Slave, Producer, Consumer",
				zoneName, zone.Kind,
			)
		}
	}

	// Validate RRsets
	c.validateRRsets(zoneName, zone.RRsets, errs)
}

func (c *Config) validateRRsets(zoneName string, rrsets []RRsetInput, errs *ValidationError) {
	seenRRsets := make(map[string]bool)

	for i, rrset := range rrsets {
		rrsetID := fmt.Sprintf("zone %q, rrset[%d] (%s/%s)", zoneName, i, rrset.Name, rrset.Type)

		// NS records must be managed via nameservers property
		if strings.EqualFold(rrset.Type, "NS") {
			errs.Add("%s: NS records must be managed via 'nameservers' property, not in rrsets", rrsetID)
			continue
		}

		// SOA records cannot be managed
		if strings.EqualFold(rrset.Type, "SOA") {
			errs.Add("%s: SOA records are managed by PowerDNS and cannot be specified", rrsetID)
			continue
		}

		if rrset.Name == "" {
			errs.Add("%s: name is required", rrsetID)
		}

		if rrset.Type == "" {
			errs.Add("%s: type is required", rrsetID)
		}

		// Check for duplicate RRsets
		key := fmt.Sprintf("%s/%s", strings.ToLower(rrset.Name), strings.ToUpper(rrset.Type))
		if seenRRsets[key] {
			errs.Add("%s: duplicate RRset definition", rrsetID)
		}
		seenRRsets[key] = true

		// Validate records
		records, err := normalizeRecords(rrset.Records)
		if err != nil {
			errs.Add("%s: %v", rrsetID, err)
			continue
		}

		if len(records) == 0 {
			errs.Add("%s: at least one record is required", rrsetID)
		}

		for j, rec := range records {
			if rec.Content == "" {
				errs.Add("%s, record[%d]: content cannot be empty", rrsetID, j)
			}
		}
	}
}

// NormalizeZone applies defaults and normalizes the zone configuration.
func (z *Zone) NormalizeZone() {
	if z.Kind == "" {
		z.Kind = "Native"
	}
}

// NormalizeRRsets normalizes RRsets by applying defaults and parsing records.
func (z *Zone) NormalizeRRsets() ([]RRset, error) {
	var rrsets []RRset

	for _, input := range z.RRsets {
		records, err := normalizeRecords(input.Records)
		if err != nil {
			return nil, fmt.Errorf("rrset %s/%s: %w", input.Name, input.Type, err)
		}

		ttl := uint32(300) // Default TTL
		if input.TTL != nil {
			ttl = *input.TTL
		}

		rrsets = append(rrsets, RRset{
			Name:    input.Name,
			Type:    strings.ToUpper(input.Type),
			TTL:     ttl,
			Records: records,
			Comment: input.Comment,
		})
	}

	return rrsets, nil
}

// normalizeRecords converts various record input formats to normalized []Record.
func normalizeRecords(input interface{}) ([]Record, error) {
	if input == nil {
		return nil, nil
	}

	switch v := input.(type) {
	case string:
		// Single string value
		return []Record{{Content: v, Disabled: false}}, nil

	case []interface{}:
		// List of mixed values
		return normalizeRecordsList(v)

	case map[string]interface{}:
		// Single object
		rec, err := parseRecordMap(v)
		if err != nil {
			return nil, err
		}
		return []Record{rec}, nil

	default:
		return nil, fmt.Errorf("unsupported records type %T", input)
	}
}

func normalizeRecordsList(items []interface{}) ([]Record, error) {
	var records []Record
	for i, item := range items {
		switch r := item.(type) {
		case string:
			records = append(records, Record{Content: r, Disabled: false})
		case map[string]interface{}:
			rec, err := parseRecordMap(r)
			if err != nil {
				return nil, fmt.Errorf("record[%d]: %w", i, err)
			}
			records = append(records, rec)
		default:
			return nil, fmt.Errorf("record[%d]: unsupported type %T", i, item)
		}
	}
	return records, nil
}

func parseRecordMap(m map[string]interface{}) (Record, error) {
	rec := Record{Disabled: false} // Default disabled to false

	if content, ok := m["content"]; ok {
		if s, ok := content.(string); ok {
			rec.Content = s
		} else {
			return Record{}, fmt.Errorf("content must be a string")
		}
	}

	if disabled, ok := m["disabled"]; ok {
		if b, ok := disabled.(bool); ok {
			rec.Disabled = b
		} else {
			return Record{}, fmt.Errorf("disabled must be a boolean")
		}
	}

	if comment, ok := m["comment"]; ok {
		if s, ok := comment.(string); ok {
			rec.Comment = s
		} else {
			return Record{}, fmt.Errorf("comment must be a string")
		}
	}

	return rec, nil
}

// CanonicalZoneName ensures zone name ends with a dot.
func CanonicalZoneName(name string) string {
	if !strings.HasSuffix(name, ".") {
		return name + "."
	}
	return name
}
