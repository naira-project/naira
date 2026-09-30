-- Schema for the catalog graph and plugin-run operations.
--
-- Applied idempotently (CREATE TABLE IF NOT EXISTS / CREATE INDEX IF NOT
-- EXISTS) on every process start. There is no migration tool

CREATE TABLE IF NOT EXISTS nodes (
    kind TEXT NOT NULL,
    path TEXT NOT NULL,
    PRIMARY KEY (kind, path)
);

CREATE TABLE IF NOT EXISTS node_plugin_claims (
    node_kind   TEXT NOT NULL,
    node_path   TEXT NOT NULL,
    plugin_name TEXT NOT NULL,
    snapshot_id UUID NOT NULL,
    properties  JSONB NOT NULL DEFAULT '{}',
    PRIMARY KEY (node_kind, node_path, plugin_name),
    FOREIGN KEY (node_kind, node_path) 
        REFERENCES nodes (kind, path) 
        ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS relations (
    kind      TEXT NOT NULL,
    from_kind TEXT NOT NULL,
    from_path TEXT NOT NULL,
    to_kind   TEXT NOT NULL,
    to_path   TEXT NOT NULL,
    PRIMARY KEY (kind, from_kind, from_path, to_kind, to_path)
);

CREATE TABLE IF NOT EXISTS relation_plugin_claims (
    relation_kind TEXT NOT NULL,
    from_kind     TEXT NOT NULL,
    from_path     TEXT NOT NULL,
    to_kind       TEXT NOT NULL,
    to_path       TEXT NOT NULL,
    plugin_name   TEXT NOT NULL,
    snapshot_id   UUID NOT NULL,
    properties    JSONB NOT NULL DEFAULT '{}',
    PRIMARY KEY (relation_kind, from_kind, from_path, to_kind, to_path, plugin_name),
    FOREIGN KEY (relation_kind, from_kind, from_path, to_kind, to_path)
        REFERENCES relations (kind, from_kind, from_path, to_kind, to_path) 
        ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS operations (
    name               TEXT PRIMARY KEY,
    plugin             TEXT NOT NULL,
    state              TEXT NOT NULL,
    start_time         TIMESTAMPTZ,
    end_time           TIMESTAMPTZ,
    error_message      TEXT,
    nodes_upserted     INT NOT NULL DEFAULT 0,
    relations_upserted INT NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL
);

-- TODO: think if needed
-- CREATE INDEX IF NOT EXISTS idx_operations_plugin_created_at ON operations (plugin, created_at DESC);
-- CREATE INDEX IF NOT EXISTS idx_operations_created_at ON operations (created_at);
