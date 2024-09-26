package main

import (
	"flag"
	"os"
	"testing"
	"time"
)

func TestParseConfig(t *testing.T) {
	// Save the original command-line arguments and defer restoring them
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Save the original environment variables and defer restoring them
	origTidyUser := os.Getenv("TIDYDNS_USER")
	origTidyPass := os.Getenv("TIDYDNS_PASS")
	defer func() {
		os.Setenv("TIDYDNS_USER", origTidyUser)
		os.Setenv("TIDYDNS_PASS", origTidyPass)
	}()

	// Set up test cases
	tests := []struct {
		name           string
		args           []string
		envUser        string
		envPass        string
		expectedConfig *config
		expectError    bool
	}{
		{
			name:    "default values",
			args:    []string{"cmd"},
			envUser: "testuser",
			envPass: "testpass",
			expectedConfig: &config{
				logLevel:           "info",
				logFormat:          "text",
				tidyEndpoint:       "",
				readTimeout:        5 * time.Second,
				writeTimeout:       10 * time.Second,
				zoneUpdateInterval: 10 * time.Minute,
				tidyUsername:       "testuser",
				tidyPassword:       "testpass",
			},
			expectError: false,
		},
		{
			name:    "custom values",
			args:    []string{"cmd", "--log-level=debug", "--log-format=json", "--tidydns-endpoint=http://example.com", "--read-timeout=3s", "--write-timeout=6s", "--zone-update-interval=15m"},
			envUser: "customuser",
			envPass: "custompass",
			expectedConfig: &config{
				logLevel:           "debug",
				logFormat:          "json",
				tidyEndpoint:       "http://example.com",
				readTimeout:        3 * time.Second,
				writeTimeout:       6 * time.Second,
				zoneUpdateInterval: 15 * time.Minute,
				tidyUsername:       "customuser",
				tidyPassword:       "custompass",
			},
			expectError: false,
		},
		{
			name:           "invalid duration",
			args:           []string{"cmd", "--zone-update-interval=invalid"},
			envUser:        "testuser",
			envPass:        "testpass",
			expectedConfig: nil,
			expectError:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set command-line arguments
			os.Args = tt.args

			// Set environment variables
			os.Setenv("TIDYDNS_USER", tt.envUser)
			os.Setenv("TIDYDNS_PASS", tt.envPass)

			// Reset the flag package to avoid conflicts
			flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

			// Call parseConfig
			cfg, err := parseConfig()

			// Check for errors
			if tt.expectError {
				if err == nil {
					t.Fatalf("expected an error but got none")
				}
				return
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}

			// Compare the result with the expected config
			if cfg.logLevel != tt.expectedConfig.logLevel ||
				cfg.logFormat != tt.expectedConfig.logFormat ||
				cfg.tidyEndpoint != tt.expectedConfig.tidyEndpoint ||
				cfg.readTimeout != tt.expectedConfig.readTimeout ||
				cfg.writeTimeout != tt.expectedConfig.writeTimeout ||
				cfg.zoneUpdateInterval != tt.expectedConfig.zoneUpdateInterval ||
				cfg.tidyUsername != tt.expectedConfig.tidyUsername ||
				cfg.tidyPassword != tt.expectedConfig.tidyPassword {
				t.Errorf("expected config %+v, but got %+v", tt.expectedConfig, cfg)
			}
		})
	}
}
