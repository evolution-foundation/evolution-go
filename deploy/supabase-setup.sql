-- WorkiDigital / Evolution Go: run once on a dedicated Supabase project, with admin privileges.
-- The Evolution Go login/license/WhatsApp sessions remain managed by the original project.
-- DO NOT expose these schemas in Supabase Data API. Back up evo_auth regularly.
BEGIN;
CREATE SCHEMA IF NOT EXISTS evo_app;
CREATE SCHEMA IF NOT EXISTS evo_auth;
CREATE SCHEMA IF NOT EXISTS evo_bridge;
REVOKE ALL ON SCHEMA evo_app, evo_auth, evo_bridge FROM PUBLIC;
REVOKE ALL ON SCHEMA evo_app, evo_auth, evo_bridge FROM anon, authenticated;
CREATE TABLE IF NOT EXISTS evo_bridge.commands (
    id uuid PRIMARY KEY,
    method text NOT NULL CHECK (method IN ('GET','POST','PUT','PATCH','DELETE')),
    request_path text NOT NULL,
    request_body bytea NOT NULL DEFAULT '\x'::bytea,
    request_sha256 text NOT NULL,
    auth_ciphertext bytea NOT NULL,
    state text NOT NULL DEFAULT 'queued'
      CHECK (state IN ('queued','processing','finished')),
    response_code integer,
    response_body bytea,
    response_type text,
    created_at timestamptz NOT NULL DEFAULT now(),
    started_at timestamptz,
    finished_at timestamptz
);
CREATE INDEX IF NOT EXISTS evo_bridge_commands_queue_idx
 ON evo_bridge.commands (created_at) WHERE state = 'queued';
REVOKE ALL ON ALL TABLES IN SCHEMA evo_bridge FROM PUBLIC, anon, authenticated;

-- Store WhatsApp media only in a private bucket.
INSERT INTO storage.buckets (id, name, public)
VALUES ('evolution-media','evolution-media',false)
ON CONFLICT (id) DO UPDATE SET public = false;
COMMIT;
