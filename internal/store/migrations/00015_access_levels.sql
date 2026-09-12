-- +goose Up
-- Dashboard roles now carry a level: viewer (read only), support (also acts
-- on tickets) or admin (everything except changing who has access). Roles
-- that were already set up keep full access.
ALTER TABLE guild_settings ADD COLUMN dashboard_roles JSONB NOT NULL DEFAULT '[]';
UPDATE guild_settings
SET dashboard_roles = (SELECT COALESCE(jsonb_agg(jsonb_build_object('role_id', r::text, 'level', 'admin')), '[]')
                       FROM unnest(dashboard_role_ids) r)
WHERE cardinality(dashboard_role_ids) > 0;
ALTER TABLE guild_settings DROP COLUMN dashboard_role_ids;

-- +goose Down
ALTER TABLE guild_settings ADD COLUMN dashboard_role_ids BIGINT[] NOT NULL DEFAULT '{}';
UPDATE guild_settings
SET dashboard_role_ids = (SELECT COALESCE(array_agg((r->>'role_id')::bigint), '{}')
                          FROM jsonb_array_elements(dashboard_roles) r);
ALTER TABLE guild_settings DROP COLUMN dashboard_roles;
