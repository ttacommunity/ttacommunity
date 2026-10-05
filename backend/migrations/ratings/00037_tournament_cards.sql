-- Each tournament's card, as the Hall of Fame and Every Tournament show it and
-- its page's header repeats it: an icon, a short description and an
-- information box. Admin edits them; the four the site described already
-- start with that text.

-- +goose Up
CREATE TABLE tournament_cards (
    tournament text PRIMARY KEY,
    icon       text NOT NULL DEFAULT '' CHECK (length(icon) <= 16),
    tagline    text NOT NULL DEFAULT '' CHECK (length(tagline) <= 60),
    summary    text NOT NULL DEFAULT '' CHECK (length(summary) <= 1000)
);
GRANT SELECT, INSERT, UPDATE, DELETE ON tournament_cards TO ratings_admin;
INSERT INTO tournament_cards (tournament, icon, tagline, summary) VALUES
    ($c$international$c$, $c$🌐$c$, $c$3/4-Player League$c$,
     $c$The premier 3/4-player competitive league in Through the Ages. Structured across hierarchical skill divisions (Grandmaster, Platinum, Gold, Silver, Bronze, Wood) with quarterly promotion and relegation.$c$),
    ($c$intermezzo$c$, $c$⚔️$c$, $c$3-Player League$c$,
     $c$The premier 3-player competitive league.$c$),
    ($c$royal_league$c$, $c$👑$c$, $c$2-Player Duel$c$,
     $c$The premier 2-player recurrent championship.$c$),
    ($c$worlds$c$, $c$🏆$c$, $c$Annual World Championship$c$,
     $c$The pinnacle of competitive Through the Ages. An annual championship bringing together the top qualified competitors worldwide across multi-stage preliminary groups, playoff brackets, and championship finals. (Note: historical format regulations have evolved across editions, and future editions may introduce new architectures).$c$);

-- +goose Down
REVOKE SELECT, INSERT, UPDATE, DELETE ON tournament_cards FROM ratings_admin;
DROP TABLE tournament_cards;
