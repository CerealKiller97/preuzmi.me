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
# Distroless: no shell and no package manager, so a far smaller attack surface
# than Alpine. The base image already ships ca-certificates (outbound HTTPS to
# the providers and IMAP-over-TLS both need a trust store) and tzdata (check_until
# and the refresh window compare against local calendar dates), so there is
# nothing to install. :nonroot runs the process as an unprivileged user (uid
# 65532) — the mounted download_path must therefore be writable by that user.
FROM gcr.io/distroless/base-debian13:nonroot

WORKDIR /app

COPY --from=build /preuzmi /app/preuzmi
COPY templates ./templates
COPY assets ./assets

EXPOSE 5500
ENTRYPOINT ["/app/preuzmi"]
CMD ["serve"]
