-- A webhook URL's path is its secret: the audit log keeps only the webhook's id, as "webhook <id>".
-- +goose Up
UPDATE admin_audit
SET detail = jsonb_set(detail, '{url}', to_jsonb('webhook ' || substring(detail->>'url' FROM '/api/webhooks/([0-9]+)/')))
WHERE action IN ('notification.endpoint.add', 'notification.endpoint.set') AND detail->>'url' LIKE 'https://%/api/webhooks/%';

-- +goose Down
-- irreversible: the URLs are gone, by design; going down leaves the ids.
SELECT 1;
