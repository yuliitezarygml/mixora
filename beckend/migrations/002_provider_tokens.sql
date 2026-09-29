CREATE TABLE provider_tokens (
 provider text PRIMARY KEY,
 encrypted_token bytea NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now()
);
