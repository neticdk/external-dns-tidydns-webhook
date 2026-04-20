FROM alpine:3.22@sha256:310c62b5e7ca5b08167e4384c68db0fd2905dd9c7493756d356e893909057601 AS setup

RUN apk add --no-cache ca-certificates \
  && addgroup -S -g 65532 nonroot \
  && adduser -S -u 65532 -G nonroot -H -D nonroot

FROM scratch

ARG TARGETPLATFORM

COPY --from=setup /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=setup /etc/passwd /etc/passwd
COPY --from=setup /etc/group /etc/group

COPY ${TARGETPLATFORM}/external-dns-tidydns-webhook /external-dns-tidydns-webhook

USER 65532:65532

ENTRYPOINT ["/external-dns-tidydns-webhook"]
