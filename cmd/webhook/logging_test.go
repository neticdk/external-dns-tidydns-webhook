package main

import (
	"bytes"
	"testing"
)

func TestLoggingSetup(t *testing.T) {
	tests := []struct {
		name       string
		logFormat  string
		logLevel   string
		addSource  bool
		expectErr  bool
		expectText string
	}{
		{
			name:       "JSON format with info level",
			logFormat:  "json",
			logLevel:   "info",
			addSource:  false,
			expectErr:  false,
			expectText: `"level":"INFO"`,
		},
		{
			name:       "Text format with debug level",
			logFormat:  "text",
			logLevel:   "debug",
			addSource:  false,
			expectErr:  false,
			expectText: "level=DEBUG",
		},
		{
			name:       "Invalid log level",
			logFormat:  "json",
			logLevel:   "invalid",
			addSource:  false,
			expectErr:  true,
			expectText: `"level":"INFO"`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer
			out := &buf

			logger := loggingSetup(test.logFormat, test.logLevel, out, test.addSource)

			if test.expectErr {
				if buf.Len() == 0 {
					t.Errorf("Expected error log, got none")
				}
			}

			logger.Info("test log")
			logOutput := buf.String()

			if !bytes.Contains([]byte(logOutput), []byte(test.expectText)) {
				t.Errorf("Expected log output to contain %q, got %q", test.expectText, logOutput)
			}
		})
	}
}

func TestLoggingSetupWithSource(t *testing.T) {
	var buf bytes.Buffer
	out := &buf

	logger := loggingSetup("text", "info", out, true)
	logger.Info("test log with source")

	logOutput := buf.String()
	if !bytes.Contains([]byte(logOutput), []byte("logging_test.go")) {
		t.Errorf("Expected log output to contain source file info, got %q", logOutput)
	}
}
