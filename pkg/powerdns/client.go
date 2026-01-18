package powerdns

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kreigan/powerdns-zone-manager/pkg/logger"
)

// Client is a PowerDNS API client for API version 1
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	log        *logger.Logger
}

// NewClient creates a new PowerDNS client
// baseURL should be the full API URL including server path, e.g.:
// http://localhost:8081/api/v1/servers/localhost
func NewClient(baseURL, apiKey string, log *logger.Logger) *Client {
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{},
		log:        log,
	}
}

// doRequest performs an HTTP request to the PowerDNS API
func (c *Client) doRequest(ctx context.Context, method, path string, body interface{}) (*http.Response, error) {
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	url := c.baseURL + path
	c.log.Debug("HTTP %s %s", method, url)

	req, err := http.NewRequestWithContext(ctx, method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("X-API-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.log.Error("HTTP request failed: %s %s: %v", method, url, err)
		return nil, fmt.Errorf("request failed: %w", err)
	}

	c.log.Debug("HTTP %s %s -> %d %s", method, url, resp.StatusCode, resp.Status)
	return resp, nil
}

// handleError processes API error responses and logs them
func (c *Client) handleError(method, path string, resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	var apiErr APIError
	if err := json.Unmarshal(body, &apiErr); err == nil && apiErr.Error != "" {
		c.log.Error("API error: %s %s -> %d: %s", method, path, resp.StatusCode, apiErr.Error)
		return fmt.Errorf("API error (status %d): %s", resp.StatusCode, apiErr.Error)
	}

	errMsg := string(body)
	if len(errMsg) > 200 {
		errMsg = errMsg[:200] + "..."
	}
	c.log.Error("API error: %s %s -> %d: %s", method, path, resp.StatusCode, errMsg)
	return fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
}

// CreateZone creates a new DNS zone
// POST /zones
// See: https://doc.powerdns.com/authoritative/http-api/zone.html
func (c *Client) CreateZone(ctx context.Context, zone *Zone) (*Zone, error) {
	path := "/zones"
	resp, err := c.doRequest(ctx, "POST", path, zone)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return nil, c.handleError("POST", path, resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var created Zone
	if err := json.Unmarshal(body, &created); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &created, nil
}

// GetZone retrieves zone information
// GET /zones/{zone_id}
// See: https://doc.powerdns.com/authoritative/http-api/zone.html
func (c *Client) GetZone(ctx context.Context, zoneID string) (*Zone, error) {
	// Ensure zone ID ends with a dot (PowerDNS requires canonical names)
	if !strings.HasSuffix(zoneID, ".") {
		zoneID += "."
	}

	path := fmt.Sprintf("/zones/%s", zoneID)
	resp, err := c.doRequest(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // Zone not found is not an error
	}

	if resp.StatusCode != http.StatusOK {
		return nil, c.handleError("GET", path, resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var zone Zone
	if err := json.Unmarshal(body, &zone); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &zone, nil
}

// PatchZone modifies RRsets in a zone
// PATCH /zones/{zone_id}
// Creates/modifies/deletes RRsets present in the payload and their comments.
// See: https://doc.powerdns.com/authoritative/http-api/zone.html
func (c *Client) PatchZone(ctx context.Context, zoneID string, patch *ZonePatch) error {
	if !strings.HasSuffix(zoneID, ".") {
		zoneID += "."
	}

	path := fmt.Sprintf("/zones/%s", zoneID)
	resp, err := c.doRequest(ctx, "PATCH", path, patch)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return c.handleError("PATCH", path, resp)
	}

	return nil
}
