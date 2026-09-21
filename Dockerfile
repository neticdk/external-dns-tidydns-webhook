FROM alpine:3.24@sha256:294b683cb724975bec92580e1e685676bd4b50bda910ddb8c51d4cabeaec77e6 AS setup

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
