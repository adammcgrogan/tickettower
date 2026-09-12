-- +goose Up
-- Everything searchable in a message: its text plus what's in its embeds
-- (dashboard replies are embeds, and form answers are embed fields).
-- +goose StatementBegin
CREATE FUNCTION ticket_message_text(content TEXT, embeds JSONB) RETURNS TEXT
LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT concat_ws(' ', content, (
        SELECT string_agg(concat_ws(' ', e->>'title', e->>'description',
            (SELECT string_agg(f->>'value', ' ') FROM jsonb_array_elements(coalesce(e->'fields', '[]')) f)), ' ')
        FROM jsonb_array_elements(embeds) e))
$$;
-- +goose StatementEnd

-- 'simple' keeps words as typed (no stemming), so order numbers, usernames
-- and non-English text match exactly.
ALTER TABLE ticket_messages ADD COLUMN search TSVECTOR
    GENERATED ALWAYS AS (to_tsvector('simple', ticket_message_text(content, embeds))) STORED;
CREATE INDEX ticket_messages_search_idx ON ticket_messages USING GIN (search);

-- +goose Down
DROP INDEX ticket_messages_search_idx;
ALTER TABLE ticket_messages DROP COLUMN search;
DROP FUNCTION ticket_message_text(TEXT, JSONB);
