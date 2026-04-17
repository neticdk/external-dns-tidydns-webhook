# external-dns-tidydns-webhook

A [webhook provider](https://kubernetes-sigs.github.io/external-dns/latest/tutorials/webhook-provider/) for [ExternalDNS](https://github.com/kubernetes-sigs/external-dns) that manages DNS records in TidyDNS.

## Features

- Supports **A**, **CNAME**, **TXT**, and **SRV** record types
- Zone caching with configurable background refresh interval
- Domain filtering (plain suffix, exclusion lists, and regex patterns)
- OpenTelemetry traces and metrics (Prometheus `/metrics` endpoint)

## Deployment

The webhook runs as a sidecar alongside ExternalDNS. Deploy it using the [external-dns Helm chart](https://github.com/kubernetes-sigs/external-dns/tree/master/charts/external-dns):

```yaml
# values.yaml
policy: sync

provider:
  name: webhook
  webhook:
    image:
      repository: ghcr.io/neticdk/external-dns-tidydns-webhook
      tag: latest  # pin to a release tag in production
    env:
      - name: TIDYDNS_USER
        valueFrom:
          secretKeyRef:
            name: tidydns-credentials
            key: username
      - name: TIDYDNS_PASS
        valueFrom:
          secretKeyRef:
            name: tidydns-credentials
            key: password
    args:
      - --tidydns-endpoint=https://tidy.example.com/index.cgi
      - --domain-filter=example.com
```

```bash
helm install external-dns external-dns/external-dns -f values.yaml
```

> [!NOTE]
> Create the Kubernetes Secret before deploying:
> ```bash
> kubectl create secret generic tidydns-credentials \
>   --from-literal=username='<tidydns-user>' \
>   --from-literal=password='<tidydns-password>'
> ```

### Endpoints

| Port | Path | Description |
|------|------|-------------|
| 8888 | `/` | Webhook API (consumed by ExternalDNS sidecar, bound to `127.0.0.1`) |
| 8080 | `/healthz` | Health check |
| 8080 | `/metrics` | Prometheus metrics |

## Configuration

All flags can also be set via environment variables (uppercase, underscores instead of dashes, e.g. `TIDYDNS_ENDPOINT`).

| Flag | Env | Default | Description |
|------|-----|---------|-------------|
| `--tidydns-endpoint` | `TIDYDNS_ENDPOINT` | *(required)* | TidyDNS server URL |
| — | `TIDYDNS_USER` | — | TidyDNS username |
| — | `TIDYDNS_PASS` | — | TidyDNS password |
| `--zone-update-interval` | `ZONE_UPDATE_INTERVAL` | `10m` | Interval for background zone refresh |
| `--max-concurrency` | `MAX_CONCURRENCY` | `10` | Max concurrent TidyDNS API calls |
| `--domain-filter` | `DOMAIN_FILTER` | — | Limit to domains matching these suffixes (repeatable) |
| `--exclude-domains` | `EXCLUDE_DOMAINS` | — | Exclude domains matching these suffixes (repeatable) |
| `--regex-domain-filter` | `REGEX_DOMAIN_FILTER` | — | Include domains matching this regex |
| `--regex-domain-exclusion` | `REGEX_DOMAIN_EXCLUSION` | — | Exclude domains matching this regex |
| `--log-level` | `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warning`, `error` |
| `--log-format` | `LOG_FORMAT` | `logfmt` | Log format: `logfmt`, `json` |
| `--read-timeout` | `READ_TIMEOUT` | `5s` | HTTP read timeout |
| `--write-timeout` | `WRITE_TIMEOUT` | `10s` | HTTP write timeout |

### OpenTelemetry

The webhook supports OpenTelemetry tracing via standard `OTEL_*` environment variables. Set `service.version` and `deployment.environment.name` via `OTEL_RESOURCE_ATTRIBUTES`:

```yaml
env:
  - name: OTEL_EXPORTER_OTLP_ENDPOINT
    value: "http://otel-collector:4317"
  - name: OTEL_RESOURCE_ATTRIBUTES
    value: "service.version=1.0.0,deployment.environment.name=production"
```

## Development

Prerequisites: Go 1.26+, [golangci-lint](https://golangci-lint.run/)

```bash
make build        # lint, test, build
make test         # run unit tests with coverage
make race         # run tests with race detector
make lint         # run golangci-lint
make fmt          # format source code
```

### Running locally

```bash
export TIDYDNS_USER='<username>'
export TIDYDNS_PASS='<password>'

go run main.go --tidydns-endpoint='https://dnsadmin.example.com/index.cgi' \
         --log-level='debug'
```

### Releasing

Releases are managed by [GoReleaser](https://goreleaser.com/). Tag a commit and push:

```bash
git tag v1.2.3
git push origin v1.2.3
```

The CI pipeline builds binaries, creates multi-arch container images (`linux/amd64`, `linux/arm64`), and publishes them to `ghcr.io/neticdk/external-dns-tidydns-webhook`.
