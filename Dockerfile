# ---- Stage 1: Build frontend ----
FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# ---- Stage 2: Build backend ----
FROM golang:1.23-alpine AS backend-builder
WORKDIR /app
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -o healthops ./cmd/healthops

# ---- Stage 3: Runtime ----
FROM alpine:3.20
RUN apk --no-cache add ca-certificates tzdata procps bind-tools openssh-client
WORKDIR /app

# Create non-root user
RUN addgroup -g 1000 app && adduser -D -u 1000 -G app app

# Copy backend binary and config
COPY --from=backend-builder /app/healthops .
COPY --from=backend-builder /app/config ./config/

# Copy frontend dist
COPY --from=frontend-builder /app/frontend/dist ./frontend/dist/

# Ensure runtime files are owned by the non-root user.
RUN chown -R app:app /app

ENV CONFIG_PATH=/app/config/default.json
ENV FRONTEND_DIR=/app/frontend/dist

USER app
EXPOSE 8080

CMD ["./healthops"]
