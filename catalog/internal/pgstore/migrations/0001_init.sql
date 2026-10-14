-- +goose Up

CREATE TABLE nodes (
    kind TEXT NOT NULL,
    path TEXT NOT NULL,
    PRIMARY KEY (kind, path)
);

CREATE TABLE node_plugin_claims (
    node_kind   TEXT NOT NULL,
    node_path   TEXT NOT NULL,
    plugin_name TEXT NOT NULL,
    snapshot_id UUID NOT NULL,
    properties  JSONB NOT NULL DEFAULT '{}',
    PRIMARY KEY (node_kind, node_path, plugin_name),
    FOREIGN KEY (node_kind, node_path) REFERENCES nodes (kind, path) ON DELETE CASCADE
);

CREATE TABLE relations (
    kind      TEXT NOT NULL,
    from_kind TEXT NOT NULL,
    from_path TEXT NOT NULL,
    to_kind   TEXT NOT NULL,
    to_path   TEXT NOT NULL,
    PRIMARY KEY (kind, from_kind, from_path, to_kind, to_path)
);

CREATE TABLE relation_plugin_claims (
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
        REFERENCES relations (kind, from_kind, from_path, to_kind, to_path) ON DELETE CASCADE
);

CREATE TABLE operations (
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
