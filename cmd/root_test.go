package cmd

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/neticdk/go-stdlib/assert"
	"github.com/spf13/viper"
)

func TestHealthMux(t *testing.T) {
	mux := healthMux(nil)

	t.Run("healthz returns 200", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		assert.Equal(t, w.Code, http.StatusOK)
	})

	t.Run("unknown path returns 404", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		assert.Equal(t, w.Code, http.StatusNotFound)
	})
}

func TestHealthMuxWithMetrics(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "metrics_output")
	})
	mux := healthMux(handler)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	assert.Equal(t, w.Code, http.StatusOK)
	assert.Equal(t, w.Body.String(), "metrics_output")
}

func TestBuildDomainFilter(t *testing.T) {
	t.Run("no filters configured", func(t *testing.T) {
		viper.Reset()
		df, err := buildDomainFilter()
		assert.NoError(t, err)
		assert.False(t, df.IsConfigured())
	})

	t.Run("plain domain filter", func(t *testing.T) {
		viper.Reset()
		viper.Set("domain_filter", []string{"example.com"})
		df, err := buildDomainFilter()
		assert.NoError(t, err)
		assert.True(t, df.IsConfigured())
		assert.True(t, df.Match("example.com"))
		assert.True(t, df.Match("sub.example.com"))
		assert.False(t, df.Match("other.com"))
	})

	t.Run("regex domain filter", func(t *testing.T) {
		viper.Reset()
		viper.Set("regex_domain_filter", `\.example\.com$`)
		df, err := buildDomainFilter()
		assert.NoError(t, err)
		assert.True(t, df.IsConfigured())
		assert.True(t, df.Match("sub.example.com"))
	})

	t.Run("invalid regex domain filter", func(t *testing.T) {
		viper.Reset()
		viper.Set("regex_domain_filter", `[invalid`)
		_, err := buildDomainFilter()
		assert.Error(t, err)
	})

	t.Run("invalid regex domain exclusion", func(t *testing.T) {
		viper.Reset()
		viper.Set("regex_domain_exclusion", `[invalid`)
		_, err := buildDomainFilter()
		assert.Error(t, err)
	})
}

func TestInitLogger(t *testing.T) {
	tests := []struct {
		name   string
		format string
		level  string
	}{
		{"json format", "json", "info"},
		{"logfmt format", "logfmt", "debug"},
		{"unknown format defaults to json", "unknown", "info"},
		{"invalid level defaults", "json", "notavalidlevel"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Should not panic
			initLogger(tt.format, tt.level)
		})
	}
}
