# syntax=docker/dockerfile:1

# ---- build CSS ----
# Tailwind v4 scans the source it can see, so templates/ and assets/ must both
# be present for the utility classes used in the HTML to survive minification.
FROM node:24-alpine AS css
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci
COPY assets ./assets
COPY templates ./templates
RUN npm run build

# ---- build binary ----
# Pin to Go 1.26 (matches go.mod). Running the toolchain on the native
# $BUILDPLATFORM and cross-compiling to $TARGET* keeps multi-arch builds off
# QEMU — the Go compiler is far faster natively than emulated.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
WORKDIR /src

ARG TARGETOS
ARG TARGETARCH

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Bring in the minified CSS built above so it ships inside the image.
COPY --from=css /app/assets/dist ./assets/dist

# CGO off → a static binary that runs on a bare Alpine. database/schema.sql and
# ./version are embedded at compile time (go:embed), so no runtime files needed
# beyond templates/ and assets/.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w" -o /preuzmi .

# ---- runtime ----
# Alpine (not distroless) so the image can ship a cron daemon: the container runs
# the web server AND the daily `checks` pass itself, so bills download with no
# host cron. busybox already provides crond. We add ca-certificates (outbound
# HTTPS to providers + IMAP-over-TLS need a trust store) and tzdata so check_until
# and the 10:00 schedule use a real local zone. crond runs as root, so the process
# runs as root and writes download_path as root.
FROM alpine:3.22

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

# Default timezone for the 10:00 schedule and the check_until day. Override at
# run time with -e TZ=... (e.g. TZ=Europe/Ljubljana).
ENV TZ=Europe/Belgrade

COPY --from=build /preuzmi /app/preuzmi
COPY templates ./templates
COPY assets ./assets

# Built-in daily receipt check. crond fires it every day; check_until (default
# 20) decides whether a given day downloads, so the schedule needs no
# day-of-month range and check_until stays the single source of truth.
COPY deploy/crontab /etc/crontabs/root
COPY deploy/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

EXPOSE 5500
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
