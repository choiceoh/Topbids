# ── Stage 1: Frontend build ──────────────────────────────────
FROM node:22-alpine AS frontend

WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json* ./
RUN npm ci
COPY frontend/ ./
RUN npm run build
# output: /src/frontend/../backend/cmd/server/static  →  /src/backend/cmd/server/static

# ── Stage 2: Go build ───────────────────────────────────────
FROM golang:1.26-alpine AS backend

RUN apk add --no-cache git

WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./

# Copy Vite build output into embed path
COPY --from=frontend /src/backend/cmd/server/static ./cmd/server/static

RUN CGO_ENABLED=0 GOOS=linux go build -o /phaeton ./cmd/server

# ── Stage 3: Minimal runtime ────────────────────────────────
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata chromium \
    && echo 'hosts: files dns' > /etc/nsswitch.conf
RUN adduser -D -h /app phaeton

WORKDIR /app
COPY --from=backend /phaeton ./phaeton

ENV CHROME_PATH=/usr/bin/chromium-browser

USER phaeton

EXPOSE 8080

ENTRYPOINT ["./phaeton"]
