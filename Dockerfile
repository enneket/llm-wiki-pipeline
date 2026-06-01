# Stage 1: Build Go binary
FROM golang:1.25-alpine AS builder

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /llm-wiki ./cmd/llm-wiki

# Stage 2: Runtime with PostgreSQL + pgvector
FROM pgvector/pgvector:pg16

# Install CA certificates for HTTPS
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*

# Copy Go binary
COPY --from=builder /llm-wiki /usr/local/bin/llm-wiki

# Copy config files
COPY config/ /app/config/
COPY pkg/database/migrations/ /app/migrations/

# Copy entrypoint script
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

# Set environment variables
ENV POSTGRES_USER=postgres
ENV POSTGRES_PASSWORD=postgres
ENV POSTGRES_DB=llm_wiki
ENV DATABASE_URL=postgres://postgres:postgres@localhost:5432/llm_wiki?sslmode=disable

# Expose port (if needed for API)
EXPOSE 6006

# Data volume
VOLUME ["/var/lib/postgresql/data", "/app/data"]

WORKDIR /app

ENTRYPOINT ["/docker-entrypoint.sh"]
CMD ["llm-wiki", "start"]
