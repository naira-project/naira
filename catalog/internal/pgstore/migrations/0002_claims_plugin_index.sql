-- +goose Up

CREATE INDEX node_plugin_claims_plugin_snapshot_idx
    ON node_plugin_claims (plugin_name, snapshot_id);

CREATE INDEX relation_plugin_claims_plugin_snapshot_idx
    ON relation_plugin_claims (plugin_name, snapshot_id);
