# External-DNS Tidy Webhook

Webhook to enable
[External-DNS](https://github.com/kubernetes-sigs/external-dns) to talk to
[Tidy-DNS](https://www.netic.dk/).

This webhook is still work in progress and represents a minimal viable product.

## Prerequisites

For general development one should have the Golang environment installed.

For deployment only Docker is necessary.

## User Guide

Tidy username and password are provided through the environment variables
`TIDYDNS_USER` and `TIDYDNS_PASS`.

The application arguments are as follows:

- `tidydns-endpoint` Tidy DNS server addr
- `zone-update-interval` The time-duration between updating the zone information
- `log-level` Application logging level (debug, info, warn, error)

This application is strictly meant to run in a container as a sidecar to
External-DNS inside a Kubernetes environment. Refer to the External-DNS
documentaion on how to set it up correctly in this context.

Locally however the application can be built and run to verify that it can talk
to Tidy DNS server and applications could be build around it to test the webhook
endpoints. Running the application locally assuming the binary is names
`webhook` could look like the following:

```sh
export TIDYDNS_USER='<tidy username>'
export TIDYDNS_PASS='<tidy password>'
./webhook --tidydns-endpoint='https://dnsadmin.company.com/index.cgi' --zone-update-interval='10m' --log-level='info'
```

## Developer Guide

All dependencies are included in the `vendor/` directory. This makes the
repository significantly larger but also means that Go need not be installed to
build the application. Docker is the only requirement. Everything else is
present. Another benefit is that running CI pipeline potentially becomes lighter
and faster because no external dependencies needs to be downloaded before
building and running tests.

Building the image with docker requires buildx if building a multiplatform
image. An example is shown below:

```sh
export VERSION=1.2.3
export REPO_PATH='registry.company.com/username/external-dns-tidydns-webhook'
export PLATFORMS='linux/amd64,linux/arm64'
docker buildx build --platform=$PLATFORMS --tag $REPO_PATH:$VERSION --push .
```

If building for the local platform is sufficient the regular build/push commands
can be used:

```sh
export VERSION=1.2.3
export REPO_PATH='registry.company.com/username/external-dns-tidydns-webhook'
docker build --tag $REPO_PATH:$VERSION .
docker push $REPO_PATH:$VERSION
```

The application can ofcause also be built locally for testing:

```sh
go build cmd/webhook/
```

## Known Issues and Limitations

- Make better use of External DNS constructs in code, see
  [External DNS webhook](https://github.com/kubernetes-sigs/external-dns/blob/master/provider/webhook/webhook.go)
- An effort should be made to use
  [tidydns-go](https://github.com/neticdk/tidydns-go) instead of the local
  tidydns package
- So far the record types are A, AAAA and CNAME
- Needs some unit tests
- More GitHub actions
  - Unit tests
  - Relase pipeline
