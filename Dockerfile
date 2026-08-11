# Stage 1: Build architecture-independent frontend assets once on the native
# builder. The resulting static files are copied into every target backend.
FROM --platform=$BUILDPLATFORM node:22-alpine@sha256:c610fcdfb1d5b4740dd70c284ed3cb16bb857e0f7166196e36a5501df7a3aa32 AS frontend
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build Go binaries on the target platform. CGO remains native to
# linux/amd64 or linux/arm64 (under QEMU when the builder is not that target).
# Use Go 1.26 (or newer) to pick up patched std-lib (html/template XSS
# escaper bypass, net/mail quadratic concat, net/http2 frame infinite loop).
# go.mod's go directive expresses minimum source compatibility, not the
# toolchain we build with.
FROM golang:1.26-alpine@sha256:0178a641fbb4858c5f1b48e34bdaabe0350a330a1b1149aabd498d0699ff5fb2 AS backend
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_STATE=unknown
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Embed frontend build into Go binary
RUN rm -rf internal/server/frontend && \
    mkdir -p internal/server/frontend && \
    cp -r /app/web/build/* internal/server/frontend/ 2>/dev/null || true
COPY --from=frontend /app/web/build ./internal/server/frontend/
RUN VERSION="$VERSION" COMMIT="$COMMIT" BUILD_STATE="$BUILD_STATE" CGO_ENABLED=1 GOOS=linux \
    ./scripts/build-go.sh /owl-invites ./cmd/owl-invites

# Stage 3: Final image
FROM alpine:3.23@sha256:fd791d74b68913cbb027c6546007b3f0d3bc45125f797758156952bc2d6daf40
ARG VERSION=dev
ARG COMMIT=unknown
ARG SOURCE_URL=https://github.com/xxi0xx/owl-invites
LABEL org.opencontainers.image.title="Owl Invites" \
      org.opencontainers.image.description="Self-hosted invitation and RSVP management" \
      org.opencontainers.image.source="$SOURCE_URL" \
      org.opencontainers.image.revision="$COMMIT" \
      org.opencontainers.image.version="$VERSION" \
      org.opencontainers.image.licenses="MIT"
RUN apk add --no-cache ca-certificates tzdata && \
    addgroup -S -g 10001 owl-invites && \
    adduser -S -D -H -u 10001 -G owl-invites owl-invites
COPY --from=backend /owl-invites /usr/local/bin/owl-invites
RUN mkdir -p /app /data /data/uploads /run/secrets && \
    chown -R 10001:10001 /data
WORKDIR /app
USER 10001:10001
ENV UPLOADS_DIR=/data/uploads \
    HOME=/nonexistent \
    TMPDIR=/tmp
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health/ready || exit 1
ENTRYPOINT ["/usr/local/bin/owl-invites"]
