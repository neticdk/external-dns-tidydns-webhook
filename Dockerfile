# syntax=docker/dockerfile:1@sha256:b6afd42430b15f2d2a4c5a02b919e98a525b785b1aaff16747d2f623364e39b6

FROM golang:alpine@sha256:8b6b77a5e6a9dda591e864e1a2856d436d94219befa5f54d7ce76d2a77cc7a06 AS builder
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
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -ldflags="-s -w" -o /webhook ./cmd/webhook

# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:nonroot@sha256:e8a4044e0b4ae4257efa45fc026c0bc30ad320d43bd4c1a7d5271bd241e386d0 AS final

COPY --from=builder /webhook /
USER 65532:65532
ENTRYPOINT [ "/webhook" ]
