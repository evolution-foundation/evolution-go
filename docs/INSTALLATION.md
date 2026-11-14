# Docker Installation Guide

This guide deploys the official Evolution Go Docker image with Docker Compose and PostgreSQL. It follows the deployment structure published in `docker/examples/docker-compose.yml` for the official project.

The example below pins the image to `0.7.3`. When a newer official release is available, review its release notes before changing the tag.

## 1. Requirements

- Docker Engine with the Docker Compose plugin (Compose v2)
- At least 1 GB of RAM available to the application and database
- At least 2 GB of free disk space
- Host port 4000 available (or choose another host-side port mapping)

You do not need Go, Git, or a local image build for this installation.

## 2. Create the deployment directory

```bash
mkdir -p evolution-go
cd evolution-go
```

Create a file named `compose.yaml`:

```yaml
services:
  evolution-go:
    image: evoapicloud/evolution-go:0.7.3
    container_name: evolution-go
    restart: unless-stopped
    ports:
      - "4000:4000"
    env_file:
      - .env
    volumes:
      - evolution_data:/app/dbdata
      - evolution_logs:/app/logs
    depends_on:
      postgres:
        condition: service_healthy

  postgres:
    image: postgres:15-alpine
    container_name: evolution-postgres
    restart: unless-stopped
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: postgres
    volumes:
      - postgres_data:/var/lib/postgresql/data
      - ./init-db.sql:/docker-entrypoint-initdb.d/init-db.sql:ro
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U ${POSTGRES_USER} -d postgres"]
      interval: 10s
      timeout: 5s
      retries: 10

volumes:
  evolution_data:
  evolution_logs:
  postgres_data:
```

The API and PostgreSQL containers communicate over the Compose network. PostgreSQL is not published on a host port in this example.

## 3. Create the PostgreSQL initialization script

Create `init-db.sql` in the same directory:

```sql
CREATE DATABASE evogo_auth;
CREATE DATABASE evogo_users;
```

The official example uses this script to create the two databases referenced by Evolution Go. The PostgreSQL image runs initialization scripts only when its data directory is first initialized.

## 4. Configure environment variables

Generate two different random values:

```bash
openssl rand -hex 32
openssl rand -hex 24
```

Create a file named `.env` in the same directory:

```env
SERVER_PORT=4000
CLIENT_NAME=evolution
GLOBAL_API_KEY=PASTE_THE_API_KEY_HERE

POSTGRES_USER=postgres
POSTGRES_PASSWORD=PASTE_THE_POSTGRES_PASSWORD_HERE
POSTGRES_AUTH_DB=postgresql://postgres:PASTE_THE_POSTGRES_PASSWORD_HERE@postgres:5432/evogo_auth?sslmode=disable
POSTGRES_USERS_DB=postgresql://postgres:PASTE_THE_POSTGRES_PASSWORD_HERE@postgres:5432/evogo_users?sslmode=disable

DATABASE_SAVE_MESSAGES=false
WADEBUG=INFO
LOGTYPE=console
LOG_DIRECTORY=/app/logs
CONNECT_ON_STARTUP=false
WEBHOOK_FILES=true
OS_NAME=Linux
MINIO_ENABLED=false
```

Replace both placeholders with the generated values. The password in the two PostgreSQL URLs must match `POSTGRES_PASSWORD`. The hexadecimal password generated above avoids URL-reserved characters.

Protect the file:

```bash
chmod 600 .env
```

Never commit or publish `.env`; it contains the API key and database credentials.

## 5. Start the services

From the directory containing `compose.yaml`, `.env`, and `init-db.sql`:

```bash
docker compose pull
docker compose up -d
docker compose ps
docker compose logs -f evolution-go
```

Wait for PostgreSQL to become healthy before the API starts. On its first initialization, PostgreSQL creates the two databases using `init-db.sql`.

## 6. Open Evolution Go and activate the license

With the default port mapping:

- API: `http://localhost:4000`
- Swagger UI: `http://localhost:4000/swagger/index.html`
- Manager: `http://localhost:4000/manager/login`

In the Manager, enter the API URL and the value configured in `GLOBAL_API_KEY`, then complete the license registration and activation flow. From another device, replace `localhost` with the server's reachable LAN address or hostname. For internet access, use HTTPS through a properly configured reverse proxy.

## 7. Updating Evolution Go

The Compose file pins version `0.7.3` to avoid unexpected updates. Before upgrading, read the target release notes and back up PostgreSQL.

After changing the image tag in `compose.yaml`, run:

```bash
docker compose pull evolution-go
docker compose up -d evolution-go
docker compose logs -f evolution-go
```

## 8. Backups and persistence

The named volume `postgres_data` stores PostgreSQL data. The `evolution_data` and `evolution_logs` volumes persist application data and logs.

Create a logical PostgreSQL backup with:

```bash
docker compose exec -T postgres sh -c 'pg_dumpall -U "$POSTGRES_USER"' > evolution-go-postgres-backup.sql
```

Keep backups outside the deployment directory and verify that they can be restored.

**Do not run `docker compose down -v` during routine maintenance or updates.** The `-v` option removes named volumes and can permanently delete stored data.

## 9. Using an existing PostgreSQL server

You can use an existing PostgreSQL server instead of the Compose database service:

1. Create the `evogo_auth` and `evogo_users` databases, or arrange for them to be created by an administrator.
2. Ensure the configured database user has the required permissions.
3. Set `POSTGRES_AUTH_DB` and `POSTGRES_USERS_DB` to connection URLs reachable from the Evolution Go container.
4. Remove the `postgres` service and the `depends_on` section from `compose.yaml`.

Inside a container, `localhost` refers to that same container, not the Docker host or another database server. Use the database server's DNS name, LAN address, or Docker network alias.

Example:

```text
postgresql://DB_USER:DB_PASSWORD@DB_HOST:5432/evogo_users?sslmode=disable
```

If a password contains URL-reserved characters such as `@`, `/`, `:`, or `#`, URL-encode it before placing it in the connection URL.

## 10. Troubleshooting

### PostgreSQL does not become healthy

- Check the service state with `docker compose ps`.
- Review the database logs with `docker compose logs --tail=200 postgres`.
- Confirm that `POSTGRES_USER` and `POSTGRES_PASSWORD` are set in `.env`.

### The databases were not created

The PostgreSQL image runs `init-db.sql` only when it initializes a new, empty data directory. If the volume already existed before the script was added, the script is not run again automatically. Create the missing databases manually or use a deliberate database initialization procedure; do not delete a volume containing data to force initialization.

### API cannot connect to PostgreSQL

- Confirm that both database URLs use the hostname `postgres` when using the Compose example.
- Do not use `localhost` as the database hostname from inside the API container.
- Check PostgreSQL logs and verify the database names and credentials.

### API port already in use

Change only the host side of the port mapping, for example `8081:4000`. Keep `SERVER_PORT=4000` because that is the port inside the container. The API will then be available at `http://localhost:8081`.

### API authentication error

Confirm that the client sends the same API key configured in `GLOBAL_API_KEY`. Do not expose the key in screenshots, public logs, or support posts.

### View recent logs

```bash
docker compose logs --tail=200 evolution-go
docker compose logs --tail=200 postgres
```

---

For source builds and local development, see the [repository README](../README.md) and [COMMANDS.md](../COMMANDS.md).
