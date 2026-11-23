# Evolution Go on Vercel Daemons + Supabase — experimental phase 1

**Source**: Evolution Foundation / Evolution Go. Upstream logo, notices and licensing preserved.
**Status**: branch for integration testing; NOT production-ready.

## Architecture

- Public Vercel **api** container `cmd/vercel-api` validates `apikey` and writes JSON commands to private `evo_bridge.commands`.
- Vercel **whatsapp** Daemon starts the original Evolution Go engine with `EVO_VERCEL_DAEMON=1`. A worker within that process claims queued commands and dispatches them through the same Gin HTTP router. WhatsApp sockets and the original license heartbeat only live in the daemon.
- Supabase PostgreSQL stores the original Evolution Go tables and session crypto in `evo_app` and `evo_auth` schemas, plus queue data in `evo_bridge`.
- A PostgreSQL advisory singleton lock prevents an old and a new daemon from running WhatsApp clients at the same time; the watchdog signals shutdown if the pinned lock connection is lost.

## Setup (not yet performed)

1. Create a **dedicated** Supabase project with a reachable direct PostgreSQL connection.
2. Run `deploy/supabase-setup.sql` with sufficient privileges; verify schemas are not exposed through the Supabase REST Data API.
3. Add all environment variables from `deploy/vercel.env.example` to **production** secrets. Use long random `GLOBAL_API_KEY`.
4. Deploy this branch in a new Vercel project from the repository root after checking image builds. Daemons run only on **production** deployments during the beta; a preview does not exercise the WhatsApp engine.
5. Evolution Go requires upstream **license activation**. Its existing Manager UI is **not** publicly available through this bridge; activate according to upstream licensing, including any required first-time registration, before expecting normal commands.
6. For initial tests, connect a non-critical WhatsApp account only, confirm persistence after restart and check the Vercel Logs for a single active daemon.

## API contract

Headers: `apikey: GLOBAL_API_KEY` and `Content-Type: application/json` where applicable.

Routes: existing Evolution Go JSON routes under `/instance/`, `/send/`, `/message/`, `/group/`, `/chat/`, `/user/`, `/community/`, `/label/`, `/unlabel/`, `/newsletter/`, `/call/`, `/polls/`, `/server/`.

Excluded initially: WebSocket, media upload/download streaming, multipart, public Manager, passkey browser pairing, arbitrary proxy destinations. Max request 256 KiB; max response 1 MiB. The gateway authenticates before queuing. Original middleware still authenticates the in-process request.

The gateway waits up to 18 seconds. When the engine completes in time it replies with the upstream HTTP status/body; otherwise it returns `202` with a `job_id`. Poll `GET /bridge/jobs/{job_id}`. It is **not** safe to automatically retry a timed-out write or message without checking the job's final status. Optional `X-Idempotency-Key` deduplicates a given method+route+key (returns 409 if body differs).

Requests already marked `processing` are **never automatically replayed** after a crash: some message sends or group updates are non-idempotent. Such requests may remain in an uncertain state and require manual reconciliation. This favors safety over automatic recovery.

## Known issues before production

- Existing upstream licensing workflow needs headless first-run onboarding or a dedicated authenticated Manager proxy.
- S3 media backend is **not implemented**. The upstream MinIO adapter cannot be assumed compatible with Supabase's `/storage/v1/s3` endpoint path; write a separate adapter and private bucket policies before enabling `MINIO_ENABLED`.
- Need execute integration tests on deployed Vercel containers and Supabase. No connected Supabase project was visible when the branch was created.
- Check connection pool limits, per-region latency, memory, monitoring, billing, migrations and task deadlines with real instances.
- Limit Vercel daemon instances to one until instance partitioning/leader election is proven.
