ALTER TABLE track_catalog
    ADD COLUMN embedding_input text NOT NULL DEFAULT '',
    ADD COLUMN embedding_input_hash text NOT NULL DEFAULT '',
    ADD COLUMN embedding_available_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN embedding_claimed_until timestamptz,
    ADD COLUMN embedding_attempts integer NOT NULL DEFAULT 0,
    ADD COLUMN embedding_error text;

CREATE TABLE track_embeddings (
    track_source text NOT NULL,
    track_id text NOT NULL,
    model_version text NOT NULL,
    dimensions integer NOT NULL CHECK (dimensions > 0),
    content_hash text NOT NULL,
    embedding vector(768) NOT NULL,
    embedded_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (track_source, track_id),
    FOREIGN KEY (track_source, track_id)
        REFERENCES track_catalog(track_source, track_id) ON DELETE CASCADE
);

CREATE INDEX track_embeddings_cosine_idx
    ON track_embeddings USING hnsw (embedding vector_cosine_ops);

CREATE INDEX track_catalog_embedding_work_idx
    ON track_catalog (embedding_available_at, embedding_claimed_until, updated_at);
