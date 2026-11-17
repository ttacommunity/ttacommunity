-- Reddit galleries as a kind of post of their own, which were images until now: a feed that sends images sends
-- galleries too.
-- +goose Up
ALTER TABLE reddit_posts DROP CONSTRAINT reddit_posts_kind_check,
    ADD CONSTRAINT reddit_posts_kind_check CHECK (kind IN ('text', 'image', 'gallery', 'video', 'link'));
ALTER TABLE reddit_feeds DROP CONSTRAINT reddit_feeds_kinds_check,
    ADD CONSTRAINT reddit_feeds_kinds_check CHECK (kinds <@ ARRAY['text', 'image', 'gallery', 'video', 'link']),
    ALTER COLUMN kinds SET DEFAULT ARRAY['text', 'image', 'gallery', 'video', 'link'];
UPDATE reddit_feeds SET kinds = array_append(kinds, 'gallery') WHERE 'image' = ANY (kinds);

-- +goose Down
UPDATE reddit_posts SET kind = 'image' WHERE kind = 'gallery';
UPDATE reddit_feeds SET kinds = array_remove(kinds, 'gallery');
ALTER TABLE reddit_posts DROP CONSTRAINT reddit_posts_kind_check,
    ADD CONSTRAINT reddit_posts_kind_check CHECK (kind IN ('text', 'image', 'video', 'link'));
ALTER TABLE reddit_feeds DROP CONSTRAINT reddit_feeds_kinds_check,
    ADD CONSTRAINT reddit_feeds_kinds_check CHECK (kinds <@ ARRAY['text', 'image', 'video', 'link']),
    ALTER COLUMN kinds SET DEFAULT ARRAY['text', 'image', 'video', 'link'];
