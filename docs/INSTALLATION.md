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
      - "127.0.0.1:4000:4000"
    environment:
      SERVER_PORT: "4000"
      CLIENT_NAME: "evolution"
      GLOBAL_API_KEY: ${GLOBAL_API_KEY:?Set GLOBAL_API_KEY in .env}
      POSTGRES_AUTH_DB: "postgresql://evolution_app:${APP_DB_PASSWORD}@postgres:5432/evogo_auth?sslmode=disable"
      POSTGRES_USERS_DB: "postgresql://evolution_app:${APP_DB_PASSWORD}@postgres:5432/evogo_users?sslmode=disable"
      DATABASE_SAVE_MESSAGES: "false"
      DEBUG_ENABLED: "INFO"
      LOG_TYPE: "console"
      LOG_DIRECTORY: "/app/logs"
      CONNECT_ON_STARTUP: "false"
      WEBHOOK_FILES: "true"
      OS_NAME: "Linux"
      MINIO_ENABLED: "false"
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
      APP_DB_PASSWORD: ${APP_DB_PASSWORD}
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

The API binds to the host's loopback interface, so it is not directly exposed to the LAN or internet. A reverse proxy running on the host can connect to `127.0.0.1:4000` and provide HTTPS. If you intentionally need direct LAN access, change the port mapping to `"4000:4000"`; understand that this exposes the API over plain HTTP, so do not use it across untrusted networks.

The API receives only its application settings and database URLs. The PostgreSQL superuser credentials are kept in the Compose environment for the database container and are not passed to Evolution Go.

## 3. Create the PostgreSQL initialization script

Create `init-db.sql` in the same directory:

```sql
\getenv app_db_password APP_DB_PASSWORD
CREATE ROLE evolution_app LOGIN PASSWORD :'app_db_password';
CREATE DATABASE evogo_auth OWNER evolution_app;
CREATE DATABASE evogo_users OWNER evolution_app;
```

The PostgreSQL image runs this script only when its data directory is first initialized. It creates a dedicated, non-superuser login for Evolution Go and makes that role the owner of both application databases. The role is not a superuser and does not have cluster-wide database-creation privileges. Both application databases are created explicitly and owned by this role.

## 4. Configure environment variables

Generate three different random values:

```bash
openssl rand -hex 32
openssl rand -hex 32
openssl rand -hex 24
```

Create a file named `.env` in the same directory:

```env
POSTGRES_USER=postgres
POSTGRES_PASSWORD=PASTE_THE_POSTGRES_SUPERUSER_PASSWORD_HERE
APP_DB_PASSWORD=PASTE_THE_APPLICATION_DATABASE_PASSWORD_HERE
GLOBAL_API_KEY=PASTE_THE_API_KEY_HERE
```

Use a different generated value for each variable. The first password is used only by PostgreSQL initialization and administration; Evolution Go connects with the dedicated `evolution_app` role. The hexadecimal passwords avoid URL-reserved characters in the database connection URLs.

Protect the file:

```bash
chmod 600 .env
```

Never commit or publish `.env`; it contains credentials and the API key.

## 5. Start the services

From the directory containing `compose.yaml`, `.env`, and `init-db.sql`:

```bash
docker compose config
docker compose pull
docker compose up -d
docker compose ps
docker compose logs -f evolution-go
```

Review the rendered configuration from `docker compose config` before starting, and do not share its output publicly because it may contain expanded secrets. Wait for PostgreSQL to become healthy before the API starts.

## 6. Open Evolution Go and activate the license

The API is bound to the host loopback interface in this secure default configuration:

- API: `http://127.0.0.1:4000`
- Swagger UI: `http://127.0.0.1:4000/swagger/index.html`
- Manager: `http://127.0.0.1:4000/manager/login`

Open the Manager locally, enter the API URL and the value configured in `GLOBAL_API_KEY`, then complete the license registration and activation flow.

For access from another device or over the internet, configure a reverse proxy that forwards to `127.0.0.1:4000` and terminates HTTPS. Do not expose the API key over plain HTTP on an untrusted network.

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

Create a protected backup directory outside the deployment directory and write the dump there:

```bash
BACKUP_DIR="$HOME/evolution-go-backups"
install -d -m 700 "$BACKUP_DIR"
umask 077
docker compose exec -T postgres sh -c 'pg_dumpall -U "$POSTGRES_USER"' \
  > "$BACKUP_DIR/evolution-go-$(date +%F-%H%M%S).sql"
```

The backup file is created with owner-only permissions. Keep backups outside the deployment directory and verify that they can be restored.

**Do not run `docker compose down -v` during routine maintenance or updates.** The `-v` option removes named volumes and can permanently delete stored data.

### Changing database passwords

Changing a value in `.env` does not change a password already stored in the persistent PostgreSQL volume. To rotate the application database password, first connect to PostgreSQL as an administrator:

```bash
docker compose exec postgres sh -c 'psql -U "$POSTGRES_USER" -d postgres'
```

At the `psql` prompt, run:

```text
\password evolution_app
```

Enter the new password when prompted. Then update `APP_DB_PASSWORD` in `.env` to the same value and recreate the API container so Compose applies the updated connection URLs:

```bash
docker compose up -d --force-recreate evolution-go
```

Do not delete the PostgreSQL volume to rotate credentials. The `POSTGRES_PASSWORD` variable also does not rotate the persisted PostgreSQL administrator password by itself; if you need to change that credential, use the same interactive `\\password postgres` procedure and update `.env` in a coordinated operation.

## 9. Using an existing PostgreSQL server

You can use an existing PostgreSQL server instead of the Compose database service:

1. Create the `evogo_auth` and `evogo_users` databases, owned by the configured application role.
2. Ensure that role owns both databases and has the required schema and table privileges. The databases should be created in advance; grant `CREATEDB` only if you intentionally want Evolution Go to create missing databases automatically.
3. Set `POSTGRES_AUTH_DB` and `POSTGRES_USERS_DB` in the Evolution Go service to connection URLs reachable from the container.
4. Remove the `postgres` service and the `depends_on` section from `compose.yaml`.

Inside a container, `localhost` refers to that same container, not the Docker host or another database server. Use the database server's DNS name, LAN address, or Docker network alias.

For a remote database, use TLS and validate the server certificate. For example:

```text
postgresql://DB_USER:DB_PASSWORD@DB_HOST:5432/evogo_users?sslmode=verify-full&sslrootcert=/path/to/ca.crt
```

Make the CA certificate available inside the Evolution Go container at the path used by `sslrootcert`. Do not use `sslmode=disable` for database traffic over an untrusted network. The local Compose example uses `sslmode=disable` only because PostgreSQL is reached over the private, isolated Compose network.

If a password contains URL-reserved characters such as `@`, `/`, `:`, or `#`, URL-encode it before placing it in the connection URL.

## 10. Troubleshooting

### PostgreSQL does not become healthy

- Check the service state with `docker compose ps`.
- Review the database logs with `docker compose logs --tail=200 postgres`.
- Confirm that `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `APP_DB_PASSWORD` are set in `.env`.

### The databases or application role were not created

The PostgreSQL image runs `init-db.sql` only when it initializes a new, empty data directory. If the volume already existed before the script was added or changed, the script is not run again automatically. Create the missing role or databases manually using an administrator account; do not delete a volume containing data to force initialization.

### API cannot connect to PostgreSQL

- Confirm that both database URLs use the hostname `postgres` when using the Compose example.
- Do not use `localhost` as the database hostname from inside the API container.
- Check PostgreSQL logs and verify the database names, application role, and credentials.

### API port already in use

Change only the host side of the port mapping, for example `127.0.0.1:8081:4000`. Keep `SERVER_PORT=4000` because that is the port inside the container. The API will then be available locally at `http://127.0.0.1:8081`.

### API authentication error

Confirm that the client sends the same API key configured in `GLOBAL_API_KEY`. Do not expose the key in screenshots, public logs, or support posts.

### WhatsApp debug logging

The application code for this release reads the WhatsApp log level from `DEBUG_ENABLED`. The upstream example files use `WADEBUG`, but that name is not read by the current configuration code. This guide therefore sets `DEBUG_ENABLED=INFO` in the Compose service. Use `DEBUG`, `INFO`, `WARN`, or `ERROR` as the desired log level.

### View recent logs

```bash
docker compose logs --tail=200 evolution-go
docker compose logs --tail=200 postgres
```

---

For source builds and local development, see the [repository README](../README.md) and [COMMANDS.md](../COMMANDS.md).
