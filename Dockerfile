# Stage 1: Build frontend
FROM node:22-alpine AS frontend
WORKDIR /app/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

# Stage 2: Build Go binary
# Use Go 1.26 (or newer) to pick up patched std-lib (html/template XSS
# escaper bypass, net/mail quadratic concat, net/http2 frame infinite loop).
# go.mod's go directive expresses minimum source compatibility, not the
# toolchain we build with.
FROM golang:1.26-alpine AS backend
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
    ./scripts/build-go.sh /openrsvp ./cmd/openrsvp
RUN VERSION="$VERSION" COMMIT="$COMMIT" BUILD_STATE="$BUILD_STATE" CGO_ENABLED=1 GOOS=linux \
    ./scripts/build-go.sh /owl-invites ./cmd/owl-invites

# Stage 3: Final image
FROM alpine:3.20
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
    addgroup -S openrsvp && adduser -S openrsvp -G openrsvp
COPY --from=backend /openrsvp /usr/local/bin/openrsvp
COPY --from=backend /owl-invites /usr/local/bin/owl-invites
RUN mkdir -p /data /data/uploads && chown -R openrsvp:openrsvp /data
USER openrsvp
ENV DB_DSN=/data/openrsvp.db
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1
ENTRYPOINT ["openrsvp"]
