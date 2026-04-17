package tidydns

import (
	"time"

	gotidydns "github.com/neticdk/tidydns-go/pkg/tidydns"
	"sigs.k8s.io/external-dns/endpoint"
)

type Config struct {
	Endpoint           string
	Username           string
	Password           string
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	ZoneUpdateInterval time.Duration
	MaxConcurrency     int
	DomainFilter       endpoint.DomainFilter
}

// NewClient creates a new TidyDNS client from the given config.
func NewClient(cfg Config) gotidydns.TidyDNSClient {
	return gotidydns.New(cfg.Endpoint, cfg.Username, cfg.Password)
}
