package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestMaskSecret(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "****"},
		{"ab", "****"},
		{"abcd", "****"},
		{"abcde", "ab*de"},
		{"secretkey123", "se********23"},
		{"mysupersecretapikey", "my***************ey"},
	}

	for _, tt := range tests {
		result := MaskSecret(tt.input)
		if result != tt.expected {
			t.Errorf("MaskSecret(%q) = %q, want %q", tt.input, result, tt.expected)
		}
		// Ensure the original secret is not exposed
		if len(tt.input) > 4 && strings.Contains(result, tt.input) {
			t.Errorf("MaskSecret(%q) = %q should not contain the original secret", tt.input, result)
		}
	}
}

func TestLogger_Info(t *testing.T) {
	var buf bytes.Buffer
	log := New(Options{Verbose: false, NoColor: true})
	log.out = &buf

	log.Info("Test message %d", 42)

	output := buf.String()
	if !strings.Contains(output, "Test message 42") {
		t.Errorf("Expected output to contain 'Test message 42', got: %s", output)
	}
}

func TestLogger_Debug_Verbose(t *testing.T) {
	var buf bytes.Buffer
	log := New(Options{Verbose: true, NoColor: true})
	log.out = &buf

	log.Debug("Debug message")

	output := buf.String()
	// Debug messages no longer have [DEBUG] prefix
	if !strings.Contains(output, "Debug message") {
		t.Errorf("Expected output to contain 'Debug message', got: %s", output)
	}
}

func TestLogger_Debug_NotVerbose(t *testing.T) {
	var buf bytes.Buffer
	log := New(Options{Verbose: false, NoColor: true})
	log.out = &buf

	log.Debug("Debug message")

	output := buf.String()
	if output != "" {
		t.Errorf("Expected no output when verbose is disabled, got: %s", output)
	}
}

func TestLogger_DryRunPrefix(t *testing.T) {
	var buf bytes.Buffer
	log := New(Options{Verbose: false, NoColor: true})
	log.out = &buf
	log.SetDryRun(true)

	log.Info("Test message")

	output := buf.String()
	if !strings.HasPrefix(output, "[DRY RUN] ") {
		t.Errorf("Expected output to start with '[DRY RUN] ', got: %s", output)
	}
}

func TestLogger_JSON_Output(t *testing.T) {
	var buf bytes.Buffer
	log := New(Options{Verbose: true, JSON: true})
	log.out = &buf

	log.Info("Test message")

	output := buf.String()
	var entry LogEntry
	if err := json.Unmarshal([]byte(output), &entry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if entry.Level != "info" {
		t.Errorf("Expected level 'info', got: %s", entry.Level)
	}
	if entry.Message != "Test message" {
		t.Errorf("Expected message 'Test message', got: %s", entry.Message)
	}
	if entry.Timestamp == "" {
		t.Error("Expected timestamp to be set")
	}
}

func TestLogger_JSON_Debug(t *testing.T) {
	var buf bytes.Buffer
	log := New(Options{Verbose: true, JSON: true})
	log.out = &buf

	log.Debug("Debug message")

	output := buf.String()
	var entry LogEntry
	if err := json.Unmarshal([]byte(output), &entry); err != nil {
		t.Fatalf("Failed to parse JSON output: %v", err)
	}

	if entry.Level != "debug" {
		t.Errorf("Expected level 'debug', got: %s", entry.Level)
	}
}

func TestLogger_HTTPRequest(t *testing.T) {
	var buf bytes.Buffer
	log := New(Options{Verbose: true, NoColor: true})
	log.out = &buf

	log.HTTPRequest("GET", "http://example.com/api")

	output := buf.String()
	if !strings.Contains(output, "REQUEST") {
		t.Errorf("Expected output to contain 'REQUEST', got: %s", output)
	}
	if !strings.Contains(output, "GET") {
		t.Errorf("Expected output to contain 'GET', got: %s", output)
	}
}

func TestLogger_HTTPResponse(t *testing.T) {
	var buf bytes.Buffer
	log := New(Options{Verbose: true, NoColor: true})
	log.out = &buf

	log.HTTPResponse("GET", "http://example.com/api", 200)

	output := buf.String()
	if !strings.Contains(output, "RESPONSE") {
		t.Errorf("Expected output to contain 'RESPONSE', got: %s", output)
	}
	if !strings.Contains(output, "200") {
		t.Errorf("Expected output to contain '200', got: %s", output)
	}
}

func TestLogger_Table(t *testing.T) {
	var buf bytes.Buffer
	log := New(Options{Verbose: false, NoColor: true})
	log.out = &buf

	headers := []string{"Name", "Type", "TTL"}
	rows := [][]string{
		{"www", "A", "300"},
		{"mail", "MX", "3600"},
	}

	log.Table("Records", headers, rows)

	output := buf.String()
	if !strings.Contains(output, "Records:") {
		t.Errorf("Expected output to contain 'Records:', got: %s", output)
	}
	if !strings.Contains(output, "Name") {
		t.Errorf("Expected output to contain 'Name', got: %s", output)
	}
	if !strings.Contains(output, "www") {
		t.Errorf("Expected output to contain 'www', got: %s", output)
	}
}
