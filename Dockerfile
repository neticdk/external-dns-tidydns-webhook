FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf AS setup

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
