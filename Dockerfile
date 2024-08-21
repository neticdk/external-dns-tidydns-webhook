# syntax=docker/dockerfile:1

# Build the webhook binary
FROM golang:1.23.0-alpine3.20 AS builder
ARG COMP_TIME=""
ARG TARGETOS
ARG TARGETARCH

WORKDIR /

# Copy the code into the container
COPY go.mod .
COPY go.sum .
COPY cmd/webhook cmd/webhook
COPY vendor vendor

# CGO_ENABLED=0 makes Go statically link the binary so we can use it in a
# distroless image. GOARCH doesn't have a default value, allowing the binary
# build for the host. For example, if we call docker build in a local env with
# Apple Silicon M1 the docker BUILDPLATFORM arg will be linux/arm64. When the
# platform is Apple x86 it will be linux/amd64. Therefore, by leaving it empty
# container and binary shipped on it has the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o webhook /cmd/webhook

# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot AS application
ARG TZ

ENV TZ=${TZ}
COPY --from=builder /webhook /
USER 65532:65532
ENTRYPOINT [ "/webhook" ]
