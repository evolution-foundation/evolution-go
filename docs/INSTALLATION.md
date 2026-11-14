# Docker Installation Guide

This guide explains how to deploy the official Evolution Go image with Docker Compose and PostgreSQL. It is intended for users who want to run Evolution Go without compiling the source code.

## 1. Requirements

- Docker Engine 24 or newer
- Docker Compose plugin (Compose v2)
- At least 1 GB RAM available to the application and database
- At least 2 GB of free disk space
- A host port available for the API (default: 8080)

No Go installation or local Docker build is required.

## 2. Create the deployment folder

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
    env_file:
      - .env
    ports:
      - "8080:8080"
    depends_on:
      postgres:
        condition: service_healthy
    volumes:
      - evolution_data:/app/dbdata
      - evolution_logs:/app/logs

  postgres:
    image: postgres:15-alpine
    container_name: evolution-go-postgres
    restart: unless-stopped
    environment:
      POSTGRES_USER: ${POSTGRES_USER}
      POSTGRES_PASSWORD: ${POSTGRES_PASSWORD}
      POSTGRES_DB: postgres
    volumes:
      - postgres_data:/var/lib/postgresql/data
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

The Compose service name `postgres` is the hostname used by Evolution Go to connect to the database. PostgreSQL is not published on a host port in this example; only the API port is exposed.

## 3. Create the environment file

Generate two different random values:

```bash
# API key
openssl rand -hex 32

# PostgreSQL password
openssl rand -hex 24
```

Create a file named `.env` in the same folder:

```env
SERVER_PORT=8080
CLIENT_NAME=evolution
GLOBAL_API_KEY=PASTE_THE_API_KEY_HERE

DATABASE_SAVE_MESSAGES=false
WADEBUG=INFO
LOGTYPE=console
CONNECT_ON_STARTUP=false
WEBHOOK_FILES=true
OS_NAME=Linux

POSTGRES_USER=evolution
POSTGRES_PASSWORD=PASTE_THE_POSTGRES_PASSWORD_HERE

POSTGRES_AUTH_DB=postgresql://evolution:PASTE_THE_POSTGRES_PASSWORD_HERE@postgres:5432/evogo_auth?sslmode=disable
POSTGRES_USERS_DB=postgresql://evolution:PASTE_THE_POSTGRES_PASSWORD_HERE@postgres:5432/evogo_users?sslmode=disable
```

Replace the placeholders with the values generated above. The password in both PostgreSQL URLs must exactly match `POSTGRES_PASSWORD`. The hexadecimal password avoids reserved URL characters that would otherwise need URL encoding.

Protect the file:

```bash
chmod 600 .env
```

**Never publish or commit this file.** It contains the API key and database credentials.

## 4. Start Evolution Go

From the directory containing `compose.yaml` and `.env`:

```bash
docker compose pull
docker compose up -d
```

Check the application logs:

```bash
docker compose logs -f evolution-go
```

The application connects to PostgreSQL and initializes its configured databases and schema. You do not need to expose the PostgreSQL port to the internet for this setup.

## 5. Open the API and activate the license

- API: [http://localhost:8080](http://localhost:8080)
- Swagger: [http://localhost:8080/swagger/index.html](http://localhost:8080/swagger/index.html)
- Manager: [http://localhost:8080/manager/login](http://localhost:8080/manager/login)

In the Manager, enter the API URL and the value configured in `GLOBAL_API_KEY`, then complete the license registration and activation flow. For a remote server, replace `localhost` with the server's reachable hostname or IP address and configure HTTPS/reverse proxy as appropriate.

## 6. Check status and logs

```bash
docker compose ps
docker compose logs -f evolution-go
docker compose logs -f postgres
curl -i http://localhost:8080/server/ok
```

## 7. Updating Evolution Go

The Compose example pins version `0.7.3` to avoid unexpected updates. Before upgrading:

1. Back up the PostgreSQL data.
2. Change the image tag in `compose.yaml` to the desired published version.
3. Pull and recreate only the application service:

```bash
docker compose pull evolution-go
docker compose up -d evolution-go
docker compose logs -f evolution-go
```

You can use the `latest` tag, but pinning a version is recommended for predictable production deployments.

## 8. Backups and persistence

PostgreSQL data is stored in the named Docker volume `postgres_data`. The Compose file also declares volumes for application data and logs.

For a logical PostgreSQL backup:

```bash
docker compose exec -T postgres sh -c 'pg_dumpall -U "$POSTGRES_USER"' > evolution-go-postgres-backup.sql
```

Store the backup somewhere safe and verify that it can be restored.

**Do not use `docker compose down -v` for routine maintenance or updates.** The `-v` option deletes named volumes and can permanently remove database contents.

## 9. Using an existing PostgreSQL server

You can use an existing PostgreSQL instance instead of the PostgreSQL service in this example:

1. Create the `evogo_auth` and `evogo_users` databases, or grant the configured PostgreSQL user permission to create databases.
2. Ensure the database user can connect and perform the operations required by the application.
3. Set `POSTGRES_AUTH_DB` and `POSTGRES_USERS_DB` to connection URLs reachable **from the Evolution Go container**.
4. Remove the `postgres` service and the `depends_on` entry from `compose.yaml).

Inside a container, `localhost` refers to that same container, not the Docker host or another database server. Use the database server's DNS name, LAN address, or a Docker network alias.

Example:

```text
postgresql://DB_USER:DB_PASSWORD@DB_HOST:5432/evogo_users?sslmode=disable
```

If a password contains reserved URL characters such as `@`, `/`, `:` or `#`, URL-encode it before placing it in the connection URL.

## 10. Troubleshooting

### Database connection refused or timeout

- Check that the PostgreSQL container is healthy: `docker compose ps`.
- When using the Compose example, confirm that both database URLs use the hostname `postgres`.
- Do not use `localhost` as the database hostname from inside the API container.

### Database creation or permission error

Confirm that the configured PostgreSQL user and password are correct and that the user has permission to create the configured databases when they do not already exist.

### API port already in use

Change the host side of the port mapping, for example `8081:8080`. Keep `SERVER_PORT=8080` because that is the port inside the container. The API will then be available at `http://localhost:8081`.

### API returns an authorization error

Confirm that the client is sending the same API key configured in `GLOBAL_API_KEY`. Never expose the key in screenshots, public logs, or support posts.

### View recent logs

```bash
docker compose logs --tail=200 evolution-go
docker compose logs --tail=200 postgres
```

---

For local development and source builds, see the [repository README](../README.md), [COMMANDS.md](../COMMANDS.md), and the project Makefile.
