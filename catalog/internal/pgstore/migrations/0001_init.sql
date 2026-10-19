-- +goose Up

CREATE TABLE nodes (
    id   UUID PRIMARY KEY,
    kind VARCHAR(40) NOT NULL,
    path VARCHAR(400) NOT NULL,
    CONSTRAINT nodes_identity_key UNIQUE (kind, path)
);

CREATE TABLE node_plugin_claims (
    node_id     UUID         NOT NULL REFERENCES nodes (id),
    plugin_name VARCHAR(40)  NOT NULL,
    snapshot_id UUID  NOT NULL,
    properties  JSONB NOT NULL DEFAULT '{}',
    PRIMARY KEY (node_id, plugin_name)
);

CREATE TABLE relations (
    id           UUID PRIMARY KEY,
    kind         VARCHAR(40) NOT NULL,
    from_node_id UUID NOT NULL REFERENCES nodes (id),
    to_node_id   UUID NOT NULL REFERENCES nodes (id),
    CONSTRAINT relations_identity_key UNIQUE (kind, from_node_id, to_node_id)
);

CREATE TABLE relation_plugin_claims (
    relation_id UUID         NOT NULL REFERENCES relations (id),
    plugin_name VARCHAR(40)  NOT NULL,
    snapshot_id UUID  NOT NULL,
    properties  JSONB NOT NULL DEFAULT '{}',
    PRIMARY KEY (relation_id, plugin_name)
);

CREATE TYPE operation_state AS ENUM (
    'PENDING',
    'RUNNING',
    'SUCCEEDED',
    'FAILED',
    'INTERRUPTED'
);

CREATE TABLE operations (
    name               TEXT PRIMARY KEY,
    plugin             VARCHAR(40) NOT NULL,
    state              operation_state NOT NULL,
    start_time         TIMESTAMPTZ,
    end_time           TIMESTAMPTZ,
    error_message      TEXT,
    nodes_upserted     INT NOT NULL DEFAULT 0,
    relations_upserted INT NOT NULL DEFAULT 0,
    created_at         TIMESTAMPTZ NOT NULL
);
