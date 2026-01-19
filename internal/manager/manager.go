// Package manager orchestrates zone creation and RRset management.
package manager

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kreigan/powerdns-zone-manager/internal/config"
	"github.com/kreigan/powerdns-zone-manager/internal/logger"
	"github.com/kreigan/powerdns-zone-manager/internal/powerdns"
)

// ErrAborted is returned when user cancels the operation.
var ErrAborted = errors.New("operation aborted by user")

// PowerDNSClient defines the interface for PowerDNS operations.
type PowerDNSClient interface {
	CreateZone(ctx context.Context, zone *powerdns.Zone) (*powerdns.Zone, error)
	GetZone(ctx context.Context, zoneID string) (*powerdns.Zone, error)
	PatchZone(ctx context.Context, zoneID string, patch *powerdns.ZonePatch) error
}

// Manager manages PowerDNS zones and records.
type Manager struct {
	client      PowerDNSClient
	log         *logger.Logger
	confirmFn   ConfirmFunc
	accountName string
}

// NewManager creates a new manager.
func NewManager(client PowerDNSClient, accountName string, log *logger.Logger) *Manager {
	return &Manager{
		client:      client,
		accountName: accountName,
		log:         log,
	}
}

// ApplyOptions contains options for the Apply operation.
type ApplyOptions struct {
	DryRun      bool
	AutoConfirm bool
}

// ConfirmFunc is a function that asks for user confirmation.
type ConfirmFunc func(prompt string) bool

// ApplyResult contains the results of an Apply operation.
type ApplyResult struct {
	Errors        []error
	ZonesCreated  int
	RRsetsCreated int
	RRsetsUpdated int
	RRsetsDeleted int
}

// Apply applies the configuration to PowerDNS.
// It first fetches all existing zones, validates the config, then applies changes.
func (m *Manager) Apply(
	ctx context.Context,
	cfg *config.Config,
	opts ApplyOptions,
) (*ApplyResult, error) {
	result := &ApplyResult{}

	// Step 1: Fetch current state of all zones in config
	m.log.Info("Fetching current state of %d zone(s)...", len(cfg.Zones))
	existingZones := make(map[string]config.ZoneState)
	zoneData := make(map[string]*powerdns.Zone)

	for zoneName := range cfg.Zones {
		canonicalName := config.CanonicalZoneName(zoneName)
		m.log.Info("  Checking zone: %s", canonicalName)
		zone, err := m.client.GetZone(ctx, canonicalName)
		if err != nil {
			return nil, fmt.Errorf("failed to check zone %s: %w", zoneName, err)
		}

		if zone != nil {
			isManaged := zone.Account == m.accountName
			existingZones[canonicalName] = config.ZoneState{
				Exists:    true,
				IsManaged: isManaged,
			}
			zoneData[canonicalName] = zone
			if isManaged {
				m.log.Info("    Zone exists (managed)")
			} else {
				m.log.Info("    Zone exists (not managed, account=%q)", zone.Account)
			}
			// Show existing managed records
			m.printManagedRRsets("Current managed records", zone)
		} else {
			existingZones[canonicalName] = config.ZoneState{
				Exists:    false,
				IsManaged: false,
			}
			m.log.Info("    Zone does not exist")
		}
	}

	// Step 2: Validate configuration against current state
	m.log.Info("Validating configuration...")
	if validationErr := cfg.Validate(existingZones); validationErr != nil {
		return nil, validationErr
	}

	// Step 3: Apply changes
	for zoneName, zoneConfig := range cfg.Zones {
		zoneConfig.NormalizeZone()
		canonicalName := config.CanonicalZoneName(zoneName)
		state := existingZones[canonicalName]

		m.log.Info("Processing zone: %s", zoneName)
		err := m.applyZone(ctx, canonicalName, &zoneConfig, state, zoneData[canonicalName], opts, result)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("zone %s: %w", zoneName, err))
		}
	}

	return result, nil
}

// SetConfirmFunc sets the confirmation function for interactive prompts.
func (m *Manager) SetConfirmFunc(fn ConfirmFunc) {
	m.confirmFn = fn
}

func (m *Manager) applyZone(
	ctx context.Context,
	zoneID string,
	zoneConfig *config.Zone,
	state config.ZoneState,
	existingZone *powerdns.Zone,
	opts ApplyOptions,
	result *ApplyResult,
) error {
	if !state.Exists {
		// Create new zone
		m.log.Info("  Creating zone: %s (kind=%s)", zoneID, zoneConfig.Kind)
		if !opts.DryRun {
			zone := &powerdns.Zone{
				Name:        zoneID,
				Kind:        zoneConfig.Kind,
				Nameservers: m.normalizeNameservers(zoneConfig.Nameservers, zoneID),
				Account:     m.accountName, // Mark zone as managed
			}

			created, err := m.client.CreateZone(ctx, zone)
			if err != nil {
				return fmt.Errorf("failed to create zone: %w", err)
			}
			existingZone = created
			m.log.Debug("  Zone created successfully")
		} else {
			// In dry run, create a mock zone for RRset processing
			existingZone = &powerdns.Zone{
				Name:   zoneID,
				RRsets: []powerdns.RRset{},
			}
		}
		result.ZonesCreated++
	}

	// Apply RRsets (including NS records from nameservers property for managed zones)
	return m.applyRRsets(ctx, zoneID, zoneConfig, existingZone, state, opts, result)
}

func (m *Manager) applyRRsets(
	ctx context.Context,
	zoneID string,
	cfg *config.Zone,
	existingZone *powerdns.Zone,
	state config.ZoneState,
	opts ApplyOptions,
	result *ApplyResult,
) error {
	// Build desired RRsets (skip NS for non-managed existing zones)
	desiredRRsets, err := m.buildDesiredRRsets(zoneID, cfg, state)
	if err != nil {
		return err
	}

	// Show desired RRsets table
	m.printDesiredRRsets("Desired records from config", desiredRRsets)

	m.log.Debug("  Desired RRsets: %d, Existing RRsets: %d", len(desiredRRsets), len(existingZone.RRsets))

	// Index existing RRsets
	existingByKey := make(map[string]powerdns.RRset)
	for _, rrset := range existingZone.RRsets {
		key := rrsetKey(rrset.Name, rrset.Type)
		existingByKey[key] = rrset
	}

	var patchRRsets []powerdns.RRset

	// Process desired RRsets
	for key, desired := range desiredRRsets {
		existing, exists := existingByKey[key]

		switch {
		case !exists:
			// Create new RRset
			m.log.Info("  + Creating RRset: %s %s", desired.Name, desired.Type)
			m.logRRsetDiff(nil, &desired)
			patchRRsets = append(patchRRsets, m.createRRsetPatch(desired))
			result.RRsetsCreated++
		case m.isManaged(existing):
			// Update managed RRset if changed
			if m.shouldUpdateRRset(desired, existing) {
				m.log.Info("  ~ Updating RRset: %s %s", desired.Name, desired.Type)
				m.logRRsetDiff(&existing, &desired)
				patchRRsets = append(patchRRsets, m.createRRsetPatch(desired))
				result.RRsetsUpdated++
			} else {
				m.log.Debug("  = RRset unchanged: %s %s", desired.Name, desired.Type)
			}
		default:
			m.log.Debug("  - Skipping non-managed RRset: %s %s", existing.Name, existing.Type)
		}
	}

	// Find orphaned managed RRsets (managed RRsets not in desired state)
	for key, existing := range existingByKey {
		if m.isManaged(existing) {
			if _, desired := desiredRRsets[key]; !desired {
				// Delete orphaned managed RRset
				m.log.Info("  - Deleting orphaned RRset: %s %s", existing.Name, existing.Type)
				m.logRRsetDiff(&existing, nil)
				patchRRsets = append(patchRRsets, powerdns.RRset{
					Name:       existing.Name,
					Type:       existing.Type,
					ChangeType: "DELETE",
				})
				result.RRsetsDeleted++
			}
		}
	}

	// Apply changes
	return m.sendPatch(ctx, zoneID, patchRRsets, opts)
}

func (m *Manager) sendPatch(
	ctx context.Context,
	zoneID string,
	patchRRsets []powerdns.RRset,
	opts ApplyOptions,
) error {
	if len(patchRRsets) == 0 {
		m.log.Debug("  No RRset changes needed")
		return nil
	}

	m.log.Debug("  Applying %d RRset change(s)...", len(patchRRsets))
	if opts.DryRun {
		return nil
	}

	// Ask for confirmation before sending changes to server
	if !opts.AutoConfirm && m.confirmFn != nil {
		if !m.confirmFn("Apply these changes?") {
			return ErrAborted
		}
	}

	patch := &powerdns.ZonePatch{RRsets: patchRRsets}
	if err := m.client.PatchZone(ctx, zoneID, patch); err != nil {
		return fmt.Errorf("failed to patch zone: %w", err)
	}

	return nil
}

func (m *Manager) buildDesiredRRsets(
	zoneID string,
	cfg *config.Zone,
	state config.ZoneState,
) (map[string]powerdns.RRset, error) {
	desired := make(map[string]powerdns.RRset)

	// Add NS RRset from nameservers property if provided
	// Only if zone is new or managed (we own it)
	if len(cfg.Nameservers) > 0 {
		if state.IsManaged || !state.Exists {
			nsRecords := make([]powerdns.Record, len(cfg.Nameservers))
			for i, ns := range cfg.Nameservers {
				nsRecords[i] = powerdns.Record{
					Content:  m.normalizeNameserver(ns, zoneID),
					Disabled: false,
				}
			}

			key := rrsetKey(zoneID, "NS")
			desired[key] = powerdns.RRset{
				Name:    zoneID,
				Type:    "NS",
				TTL:     300, // Default TTL for NS records
				Records: nsRecords,
			}
		} else {
			// Zone exists but is not managed - warn about skipped nameservers
			m.log.Warn("  Skipping nameservers (zone is not managed)")
		}
	}

	// Add RRsets from config
	rrsets, err := cfg.NormalizeRRsets()
	if err != nil {
		return nil, err
	}

	for _, rrset := range rrsets {
		fqdn := m.buildFQDN(rrset.Name, zoneID)
		key := rrsetKey(fqdn, rrset.Type)

		records := make([]powerdns.Record, len(rrset.Records))
		for i, rec := range rrset.Records {
			content := rec.Content
			// Normalize TXT records: wrap in quotes if not already quoted
			if rrset.Type == "TXT" && !strings.HasPrefix(content, "\"") {
				content = fmt.Sprintf("%q", content)
			}
			records[i] = powerdns.Record{
				Content:  content,
				Disabled: rec.Disabled,
			}
		}

		desired[key] = powerdns.RRset{
			Name:    fqdn,
			Type:    rrset.Type,
			TTL:     rrset.TTL,
			Records: records,
		}
	}

	return desired, nil
}

func (m *Manager) createRRsetPatch(desired powerdns.RRset) powerdns.RRset {
	return powerdns.RRset{
		Name:       desired.Name,
		Type:       desired.Type,
		TTL:        desired.TTL,
		ChangeType: "REPLACE",
		Records:    desired.Records,
		Comments: []powerdns.Comment{
			{
				Content: "Managed by zone-manager",
				Account: m.accountName,
			},
		},
	}
}

// isManaged returns true if the RRset has at least one comment with our account.
func (m *Manager) isManaged(rrset powerdns.RRset) bool {
	for _, comment := range rrset.Comments {
		if comment.Account == m.accountName {
			return true
		}
	}
	return false
}

func (m *Manager) shouldUpdateRRset(desired, existing powerdns.RRset) bool {
	if desired.TTL != existing.TTL {
		return true
	}

	if len(desired.Records) != len(existing.Records) {
		return true
	}

	// Sort and compare records
	desiredContents := make([]string, len(desired.Records))
	existingContents := make([]string, len(existing.Records))

	for i, r := range desired.Records {
		desiredContents[i] = fmt.Sprintf("%s|%t", r.Content, r.Disabled)
	}
	for i, r := range existing.Records {
		existingContents[i] = fmt.Sprintf("%s|%t", r.Content, r.Disabled)
	}

	sort.Strings(desiredContents)
	sort.Strings(existingContents)

	for i := range desiredContents {
		if desiredContents[i] != existingContents[i] {
			return true
		}
	}

	return false
}

func (m *Manager) buildFQDN(name, zoneID string) string {
	if name == "@" {
		return zoneID
	}
	if strings.HasSuffix(name, ".") {
		return name
	}
	return fmt.Sprintf("%s.%s", name, zoneID)
}

func (m *Manager) normalizeNameservers(nameservers []string, zoneID string) []string {
	result := make([]string, len(nameservers))
	for i, ns := range nameservers {
		result[i] = m.normalizeNameserver(ns, zoneID)
	}
	return result
}

func (m *Manager) normalizeNameserver(ns, zoneID string) string {
	// If already FQDN, return as-is
	if strings.HasSuffix(ns, ".") {
		return ns
	}
	// If it's just a hostname, append zone
	return fmt.Sprintf("%s.%s", ns, zoneID)
}

func rrsetKey(name, recordType string) string {
	return fmt.Sprintf("%s/%s", strings.ToLower(name), strings.ToUpper(recordType))
}

const disabledSuffix = " [disabled]"

// formatRecord returns a formatted string for a record with optional disabled status.
func formatRecord(content string, disabled bool) string {
	if disabled {
		return content + disabledSuffix
	}
	return content
}

// logRRsetDiff logs detailed diff between existing and desired RRsets in verbose mode.
func (m *Manager) logRRsetDiff(existing, desired *powerdns.RRset) {
	switch {
	case existing == nil && desired != nil:
		m.logNewRRset(desired)
	case existing != nil && desired == nil:
		m.logDeletedRRset(existing)
	case existing != nil && desired != nil:
		m.logUpdatedRRset(existing, desired)
	}
}

func (m *Manager) logNewRRset(desired *powerdns.RRset) {
	m.log.Debug("      TTL: %d", desired.TTL)
	for _, r := range desired.Records {
		m.log.Diff("+", formatRecord(r.Content, r.Disabled))
	}
}

func (m *Manager) logDeletedRRset(existing *powerdns.RRset) {
	m.log.Debug("      TTL: %d", existing.TTL)
	for _, r := range existing.Records {
		m.log.Diff("-", formatRecord(r.Content, r.Disabled))
	}
}

func (m *Manager) logUpdatedRRset(existing, desired *powerdns.RRset) {
	if existing.TTL != desired.TTL {
		m.log.Debug("      TTL: %d -> %d", existing.TTL, desired.TTL)
	} else {
		m.log.Debug("      TTL: %d", desired.TTL)
	}

	existingRecords := make(map[string]powerdns.Record)
	for _, r := range existing.Records {
		existingRecords[r.Content] = r
	}
	desiredRecords := make(map[string]powerdns.Record)
	for _, r := range desired.Records {
		desiredRecords[r.Content] = r
	}

	// Show removed records
	for content, r := range existingRecords {
		if _, exists := desiredRecords[content]; !exists {
			m.log.Diff("-", formatRecord(content, r.Disabled))
		}
	}

	// Show added or changed records
	for content, r := range desiredRecords {
		existingR, exists := existingRecords[content]
		switch {
		case !exists:
			m.log.Diff("+", formatRecord(content, r.Disabled))
		case existingR.Disabled != r.Disabled:
			oldFmt := formatRecord(content, existingR.Disabled)
			newFmt := formatRecord(content, r.Disabled)
			m.log.Diff("~", oldFmt+" -> "+newFmt)
		}
	}
}

// printManagedRRsets displays managed RRsets in table format.
func (m *Manager) printManagedRRsets(title string, zone *powerdns.Zone) {
	var rows [][]string

	for _, rrset := range zone.RRsets {
		if m.isManaged(rrset) {
			for _, record := range rrset.Records {
				status := ""
				if record.Disabled {
					status = "disabled"
				}
				rows = append(rows, []string{
					rrset.Name,
					rrset.Type,
					fmt.Sprintf("%d", rrset.TTL),
					record.Content,
					status,
				})
			}
		}
	}

	// Sort by Type, then Name
	sortTableRows(rows)

	headers := []string{"NAME", "TYPE", "TTL", "CONTENT", "STATUS"}
	m.log.Table(title, headers, rows)
}

// printDesiredRRsets displays desired RRsets from config in table format.
func (m *Manager) printDesiredRRsets(title string, rrsets map[string]powerdns.RRset) {
	// Count total records for preallocation
	totalRecords := 0
	for _, rrset := range rrsets {
		totalRecords += len(rrset.Records)
	}

	rows := make([][]string, 0, totalRecords)

	for _, rrset := range rrsets {
		for _, record := range rrset.Records {
			status := ""
			if record.Disabled {
				status = "disabled"
			}
			rows = append(rows, []string{
				rrset.Name,
				rrset.Type,
				fmt.Sprintf("%d", rrset.TTL),
				record.Content,
				status,
			})
		}
	}

	// Sort by Type, then Name
	sortTableRows(rows)

	headers := []string{"NAME", "TYPE", "TTL", "CONTENT", "STATUS"}
	m.log.Table(title, headers, rows)
}

// sortTableRows sorts rows by Type (index 1), then Name (index 0).
func sortTableRows(rows [][]string) {
	sort.Slice(rows, func(i, j int) bool {
		if rows[i][1] != rows[j][1] {
			return rows[i][1] < rows[j][1]
		}
		return rows[i][0] < rows[j][0]
	})
}
