# The hub as one image: a static Go binary with its migrations embedded, on a
# minimal base, running as an unprivileged user. Build from this directory:
#
#   docker build -t cuckoo-hub:$(git describe --always) --build-arg VERSION=$(git describe --always) server/
#
# `make image` at the repository root does exactly that for both images.

FROM golang:1.26-alpine AS build
WORKDIR /src

# Dependencies first, so a code change does not re-download them.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o /out/cuckoo ./cmd/cuckoo

FROM alpine:3.20
# Certificates for outbound HTTPS (webhook deliveries), time zones for logs.
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -u 10001 cuckoo
COPY --from=build /out/cuckoo /usr/local/bin/cuckoo

# The media directory is a bind mount owned by this uid; see deploy/README.md.
USER cuckoo
EXPOSE 8080
# busybox wget ships with alpine, so the check needs nothing extra.
HEALTHCHECK --interval=30s --timeout=3s --start-period=15s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1
ENTRYPOINT ["cuckoo"]
