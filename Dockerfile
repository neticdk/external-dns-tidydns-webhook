# syntax=docker/dockerfile:1

ARG GO_VERSION=1.23.1
FROM golang:1.23.1 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Build the application.
# Leverage a cache mount to /go/pkg/mod/ to speed up subsequent builds.
# Leverage a bind mount to the current directory to avoid having to copy the
# source code into the container.
# CGO_ENABLED=0 makes Go statically link the binary so we can use it in a
# distroless image. GOARCH doesn't have a default value, allowing the binary
# build for the host. For example, if we call docker build in a local env with
# Apple Silicon M1 the docker BUILDPLATFORM arg will be linux/arm64. When the
# platform is Apple x86 it will be linux/amd64. Therefore, by leaving it empty
# container and binary shipped on it has the same platform.
RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,target=. \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /webhook cmd/webhook

# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot AS final

COPY --from=builder /webhook /
USER 65532:65532
ENTRYPOINT [ "/webhook" ]
