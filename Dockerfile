FROM alpine:3.21 AS release

WORKDIR /app

# Docker buildx 会在构建时自动填充这些变量
ARG TARGETOS
ARG TARGETARCH

RUN apk add --no-cache ca-certificates curl tzdata

COPY --chmod=755 komari-${TARGETOS}-${TARGETARCH} /app/komari

ENV GIN_MODE=release
ENV KOMARI_LISTEN=0.0.0.0:25774
ENV GODEBUG=disablethp=1

EXPOSE 25774 25775

CMD ["/app/komari", "server"]

# Default source build: no prebuilt frontend or backend artifacts required.
FROM node:24-bookworm-slim AS frontend
WORKDIR /src
COPY komari-web/package.json komari-web/package-lock.json ./komari-web/
RUN npm ci --prefix komari-web
COPY komari-web ./komari-web
COPY scripts/embed-frontend.mjs ./scripts/embed-frontend.mjs
COPY scripts/share-archive.mjs ./scripts/share-archive.mjs
RUN npm run build --prefix komari-web && node scripts/embed-frontend.mjs

FROM golang:1.25.0-bookworm AS backend
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/web/public/defaultTheme ./web/public/defaultTheme
RUN CGO_ENABLED=1 go build -trimpath -ldflags="-s -w" -o /out/komari .

FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /app
COPY --from=backend /out/komari /app/komari
ENV GIN_MODE=release KOMARI_LISTEN=0.0.0.0:25774 GODEBUG=disablethp=1
EXPOSE 25774 25775
CMD ["/app/komari", "server"]
