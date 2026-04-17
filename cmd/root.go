package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"syscall"
	"time"

	"github.com/neticdk/external-dns-tidydns-webhook/internal/tidydns"
	"github.com/neticdk/go-common/pkg/log"
	"github.com/neticdk/go-common/pkg/telemetry"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/provider/webhook/api"
)

var rootCmd = &cobra.Command{
	Use:   "external-dns-tidydns-webhook",
	Short: "A webhook provider for ExternalDNS using TidyDNS",
	Long:  `A webhook provider for ExternalDNS that manages DNS records in TidyDNS.`,

	RunE: func(_ *cobra.Command, _ []string) error {
		initLogger(viper.GetString("log_format"), viper.GetString("log_level"))
		logger := slog.Default()

		// Initialize OpenTelemetry
		shutdownTelemetry, err := telemetry.ConfigureTelemetry(
			0,
			"external-dns-tidydns-webhook",
			telemetry.WithoutMetricsServer(),
			telemetry.WithEnvConfig(),
		)
		if err != nil {
			logger.Warn("failed to initialize telemetry, continuing without", "error", err)
		}

		if viper.GetString("tidydns_endpoint") == "" {
			return fmt.Errorf("invalid configuration: tidydns-endpoint is required")
		}

		domainFilter, err := buildDomainFilter()
		if err != nil {
			return err
		}

		cfg := tidydns.Config{
			Endpoint:           viper.GetString("tidydns_endpoint"),
			Username:           viper.GetString("tidydns_user"),
			Password:           viper.GetString("tidydns_pass"),
			ReadTimeout:        viper.GetDuration("read_timeout"),
			WriteTimeout:       viper.GetDuration("write_timeout"),
			ZoneUpdateInterval: viper.GetDuration("zone_update_interval"),
			MaxConcurrency:     viper.GetInt("max_concurrency"),
			DomainFilter:       domainFilter,
		}

		client := tidydns.NewClient(cfg)

		provider, err := tidydns.NewProvider(client, cfg)
		if err != nil {
			return fmt.Errorf("creating provider: %w", err)
		}
		defer provider.Stop()

		logger.Info("Starting TidyDNS Webhook", "endpoint", cfg.Endpoint)

		// Start the webhook API server for external-dns on localhost:8888
		go api.StartHTTPApi(provider, nil, cfg.ReadTimeout, cfg.WriteTimeout, "127.0.0.1:8888")

		// Start health/metrics server on 0.0.0.0:8080
		healthServer := &http.Server{
			Addr:         "0.0.0.0:8080",
			Handler:      healthMux(promhttp.Handler()),
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		}

		go func() {
			logger.Info("Starting health/metrics server", "addr", healthServer.Addr)
			if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("health server error", "error", err)
			}
		}()

		// Wait for shutdown signal
		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		<-ctx.Done()
		logger.Info("Received shutdown signal")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := healthServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("health server shutdown: %w", err)
		}

		if shutdownTelemetry != nil {
			if err := shutdownTelemetry(shutdownCtx); err != nil {
				logger.Warn("telemetry shutdown error", "error", err)
			}
		}

		logger.Info("Server exited gracefully")
		return nil
	},
}

func init() {
	viper.AutomaticEnv()

	rootCmd.Flags().String("log-level", "info", "Set the level of logging (options: debug, info, warning, error)")
	_ = viper.BindPFlag("log_level", rootCmd.Flags().Lookup("log-level"))

	rootCmd.Flags().String("log-format", "logfmt", "The format in which log messages are printed (options: logfmt, json)")
	_ = viper.BindPFlag("log_format", rootCmd.Flags().Lookup("log-format"))

	rootCmd.Flags().String("tidydns-endpoint", "", "TidyDNS server endpoint")
	_ = viper.BindPFlag("tidydns_endpoint", rootCmd.Flags().Lookup("tidydns-endpoint"))

	rootCmd.Flags().Duration("read-timeout", 5*time.Second, "Read timeout in duration format")
	_ = viper.BindPFlag("read_timeout", rootCmd.Flags().Lookup("read-timeout"))

	rootCmd.Flags().Duration("write-timeout", 10*time.Second, "Write timeout in duration format")
	_ = viper.BindPFlag("write_timeout", rootCmd.Flags().Lookup("write-timeout"))

	rootCmd.Flags().Duration("zone-update-interval", 10*time.Minute, "The interval at which to update zone information (e.g. 1h32m)")
	_ = viper.BindPFlag("zone_update_interval", rootCmd.Flags().Lookup("zone-update-interval"))

	rootCmd.Flags().Int("max-concurrency", 10, "Maximum number of concurrent TidyDNS API calls during apply")
	_ = viper.BindPFlag("max_concurrency", rootCmd.Flags().Lookup("max-concurrency"))

	rootCmd.Flags().StringSlice("domain-filter", nil, "Limit to domains matching these suffixes (repeatable)")
	_ = viper.BindPFlag("domain_filter", rootCmd.Flags().Lookup("domain-filter"))

	rootCmd.Flags().StringSlice("exclude-domains", nil, "Exclude domains matching these suffixes (repeatable)")
	_ = viper.BindPFlag("exclude_domains", rootCmd.Flags().Lookup("exclude-domains"))

	rootCmd.Flags().String("regex-domain-filter", "", "Include domains matching this regex (overrides --domain-filter)")
	_ = viper.BindPFlag("regex_domain_filter", rootCmd.Flags().Lookup("regex-domain-filter"))

	rootCmd.Flags().String("regex-domain-exclusion", "", "Exclude domains matching this regex")
	_ = viper.BindPFlag("regex_domain_exclusion", rootCmd.Flags().Lookup("regex-domain-exclusion"))
}

// Execute runs the root command and returns the exit code
func Execute(version string) int {
	rootCmd.Version = version
	if err := rootCmd.Execute(); err != nil {
		return 1
	}
	return 0
}

// buildDomainFilter constructs an endpoint.DomainFilter from CLI flags using
// the same options pattern as external-dns.
func buildDomainFilter() (endpoint.DomainFilter, error) {
	var regexInclude, regexExclude *regexp.Regexp
	var err error

	if s := viper.GetString("regex_domain_filter"); s != "" {
		regexInclude, err = regexp.Compile(s)
		if err != nil {
			return endpoint.DomainFilter{}, fmt.Errorf("invalid --regex-domain-filter: %w", err)
		}
	}
	if s := viper.GetString("regex_domain_exclusion"); s != "" {
		regexExclude, err = regexp.Compile(s)
		if err != nil {
			return endpoint.DomainFilter{}, fmt.Errorf("invalid --regex-domain-exclusion: %w", err)
		}
	}

	return *endpoint.NewDomainFilterWithOptions(
		endpoint.WithDomainFilter(viper.GetStringSlice("domain_filter")),
		endpoint.WithDomainExclude(viper.GetStringSlice("exclude_domains")),
		endpoint.WithRegexDomainFilter(regexInclude),
		endpoint.WithRegexDomainExclude(regexExclude),
	), nil
}

func healthMux(metricsHandler http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	if metricsHandler != nil {
		mux.Handle("GET /metrics", metricsHandler)
	}
	return mux
}

const defaultLogLevel = slog.LevelInfo

func initLogger(logFormat, logLevel string) {
	logLeveller := new(slog.LevelVar)
	if err := logLeveller.UnmarshalText([]byte(logLevel)); err != nil {
		slog.Default().Error(err.Error())
		logLeveller.Set(defaultLogLevel)
	}

	handlerOpts := slog.HandlerOptions{
		Level:     logLeveller,
		AddSource: logLeveller.Level() == slog.LevelDebug,
	}

	var h slog.Handler
	switch logFormat {
	case "json":
		h = log.NewJSONTraceIDHandler(os.Stdout, &handlerOpts)
	case "logfmt":
		h = log.NewTextTraceIDHandler(os.Stdout, &handlerOpts)
	default:
		h = log.NewJSONTraceIDHandler(os.Stdout, &handlerOpts)
	}

	logger := slog.New(h)
	slog.SetDefault(logger)

	logger.Debug("using loglevel " + logLeveller.Level().String())
}
