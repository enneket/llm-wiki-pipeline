#!/bin/bash
set -e

# Run the original postgres entrypoint in the background
# to initialize the database and start PostgreSQL
docker-entrypoint.sh postgres &
POSTGRES_PID=$!

# Wait for PostgreSQL to be ready
echo "Waiting for PostgreSQL to start..."
until pg_isready -U postgres -h localhost -q; do
  sleep 1
done
echo "PostgreSQL is ready!"

# Run migrations (all .sql files in migrations/ in order)
echo "Running migrations..."
for f in /app/migrations/*.sql; do
  if [ -f "$f" ]; then
    echo "  Running $f..."
    psql -U postgres -d llm_wiki -f "$f"
  fi
done
echo "Migrations complete!"

# Execute the main command (e.g., llm-wiki start)
echo "Starting application..."
exec "$@"
