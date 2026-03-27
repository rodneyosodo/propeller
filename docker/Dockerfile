FROM golang:1.26-alpine3.23 AS builder
ARG SVC
# TARGETARCH / TARGETVARIANT are set automatically by docker buildx.
# GOARCH / GOARM can still be passed explicitly via --build-arg to keep
# backwards-compatibility with the `make dockers` Makefile targets.
ARG TARGETARCH
ARG TARGETVARIANT
ARG GOARCH=${TARGETARCH}
ARG GOARM
ARG VERSION
ARG COMMIT
ARG TIME

WORKDIR /go/src/github.com/absmach/propeller
COPY . .
RUN apk update \
    && apk add make upx \
    && _goarm="${GOARM:-${TARGETVARIANT#v}}" \
    && make $SVC GOARCH=${GOARCH} ${_goarm:+GOARM=${_goarm}} \
    && (upx --best build/$SVC 2>/dev/null || true) \
    && mv build/$SVC /exe

FROM scratch
LABEL org.opencontainers.image.source=https://github.com/absmach/propeller
LABEL org.opencontainers.image.licenses=Apache-2.0

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=builder /exe /
ENTRYPOINT ["/exe"]
