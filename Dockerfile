FROM alpine:3.23@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40 AS setup

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
