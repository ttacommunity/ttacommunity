-- Community Leaderboard Editions: titles, badges, maintainer details, and visibility
-- managed in Admin and rendered dynamically on the site.
-- +goose Up
CREATE TABLE public.community_editions (
    slug text PRIMARY KEY,
    name text NOT NULL,
    title text NOT NULL,
    short_name text NOT NULL,
    badge text NOT NULL,
    maintainer text NOT NULL,
    maintainer_note text NOT NULL,
    year integer NOT NULL,
    counting_events integer NOT NULL DEFAULT 8,
    visible boolean NOT NULL DEFAULT true,
    sort_order integer NOT NULL DEFAULT 0
);

INSERT INTO public.community_editions (slug, name, title, short_name, badge, maintainer, maintainer_note, year, counting_events, visible, sort_order)
VALUES
    ('2026', '2026 (Rolling Q2)', 'Official Community Leaderboard • Q2 2026 Cycle', '2026 Q2', '⭐ Current Rolling', 'wolvs', 'Tracking tournament finishes under the official Best-8 system.', 2026, 8, true, 1),
    ('2025', '2025 Final Archive', 'Season 2025 Official Community Leaderboard Final Archive', '2025 Final', '🏆 2025 Final', 'wolvs', 'Official final standings for Season 2025 under the Best-8 system.', 2025, 8, true, 2),
    ('2024', '2024 Final Archive', 'Season 2024 Official Community Leaderboard Final Archive', '2024 Final', '📜 2024 Final', 'wolvs', 'Official final standings for Season 2024 under the Best-8 system.', 2024, 8, true, 3),
    ('race26', '2026 Race Tracker', 'Season 2026 Points Race Tracker (Race 26)', 'Race 26', '🏁 2026 Race', 'wolvs', 'Live seasonal points race for calendar year 2026.', 2026, 6, true, 4);

GRANT SELECT ON TABLE public.community_editions TO ratings_read, ratings_admin;
GRANT INSERT, UPDATE, DELETE ON TABLE public.community_editions TO ratings_admin;

-- +goose Down
DROP TABLE public.community_editions;
