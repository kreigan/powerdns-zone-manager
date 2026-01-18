package logger

import (
	"bytes"
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
	log := New(false)
	log.out = &buf

	log.Info("Test message %d", 42)

	output := buf.String()
	if !strings.Contains(output, "Test message 42") {
		t.Errorf("Expected output to contain 'Test message 42', got: %s", output)
	}
}

func TestLogger_Debug_Verbose(t *testing.T) {
	var buf bytes.Buffer
	log := New(true) // verbose enabled
	log.out = &buf

	log.Debug("Debug message")

	output := buf.String()
	if !strings.Contains(output, "[DEBUG]") {
		t.Errorf("Expected output to contain '[DEBUG]', got: %s", output)
	}
	if !strings.Contains(output, "Debug message") {
		t.Errorf("Expected output to contain 'Debug message', got: %s", output)
	}
}

func TestLogger_Debug_NotVerbose(t *testing.T) {
	var buf bytes.Buffer
	log := New(false) // verbose disabled
	log.out = &buf

	log.Debug("Debug message")

	output := buf.String()
	if output != "" {
		t.Errorf("Expected no output when verbose is disabled, got: %s", output)
	}
}

func TestLogger_DryRunPrefix(t *testing.T) {
	var buf bytes.Buffer
	log := New(false)
	log.out = &buf
	log.SetDryRun(true)

	log.Info("Test message")

	output := buf.String()
	if !strings.HasPrefix(output, "[DRY RUN] ") {
		t.Errorf("Expected output to start with '[DRY RUN] ', got: %s", output)
	}
}
