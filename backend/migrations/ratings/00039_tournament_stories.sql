-- A tournament's stories, as DANeo's series pages tell them: a player, a badge,
-- a title and the story. Admin writes them; the majors start with his.

-- +goose Up
CREATE TABLE tournament_stories (
    tournament  text NOT NULL,
    position    int  NOT NULL CHECK (position >= 1),
    player_name text NOT NULL CHECK (player_name <> ''),
    badge       text NOT NULL CHECK (length(badge) <= 60),
    title       text NOT NULL CHECK (title <> '' AND length(title) <= 120),
    text        text NOT NULL CHECK (text <> '' AND length(text) <= 1000),
    PRIMARY KEY (tournament, position)
);
GRANT SELECT, INSERT, UPDATE, DELETE ON tournament_stories TO ratings_admin;
INSERT INTO tournament_stories (tournament, position, player_name, badge, title, text) VALUES
    ($s$international$s$, 1, $s$Weidenbaum$s$, $s$Dynastic Record$s$,
     $s$The Golden Hexa-Crown • 6 International Titles & 9 Podiums$s$,
     $s$Grandmaster Weidenbaum holds the all-time championship record in the International Championship with 6 GM Division titles, 2 runner-up finishes, and 1 bronze across 14 competitive seasons.$s$),
    ($s$international$s$, 2, $s$Genghisip$s$, $s$Podium Benchmark$s$,
     $s$All-Time Medal Leader • 11 Top-3 Finishes & 4 Titles$s$,
     $s$Holding the all-time record for total podium finishes in the International Championship, Genghisip captured 4 Gold medals, 2 Silvers, and 5 Bronzes across 15 recorded seasons.$s$),
    ($s$international$s$, 3, $s$DANeo$s$, $s$Record Campaign$s$,
     $s$All-Time Scoring Season • 36.0 / 39.0 Points & Premier Title$s$,
     $s$DANeo authored one of the most dominant seasonal campaigns in premier 4-player history during Season 31, scoring 36.0 out of a maximum 39.0 points in the Grandmaster Division to claim the Gold crown.$s$),
    ($s$intermezzo$s$, 1, $s$Weidenbaum$s$, $s$All-Time Dynasty$s$,
     $s$Eight-Time Champion • 15 Premier Podiums$s$,
     $s$Weidenbaum stands as the most decorated champion in Intermezzo history, claiming 8 Gold medals, 3 Silver medals, and 4 Bronze medals across 17 seasons, totaling an unmatched 314 Hall of Fame points.$s$),
    ($s$intermezzo$s$, 2, $s$Martin_Pecheur$s$, $s$Triple Champion$s$,
     $s$World Champion Pedigree • 3 Titles & 8 Podiums$s$,
     $s$Martin_Pecheur captured 3 Intermezzo Championships, 3 runner-up finishes, and 2 bronze medals across 10 seasons (172 Hall of Fame points).$s$),
    ($s$royal_league$s$, 1, $s$DANeo$s$, $s$Reigning Emperor Champion$s$,
     $s$Season 9 Emperor Gold & Multi-Podium Legend$s$,
     $s$DANeo captured the Season 9 Royal League Emperor Division Championship with a 6W-1L (12.0 pts) performance, alongside two Silver Medals in Season 4 and Season 7.$s$),
    ($s$royal_league$s$, 2, $s$saru$s$, $s$Two-Time Emperor Champion$s$,
     $s$Back-to-Back Titles & Season 9 Silver$s$,
     $s$saru captured back-to-back Royal League Premier Emperor Division titles in Season 7 and Season 8, followed by a Silver finish in Season 9 (clinching on H2H tiebreaker).$s$),
    ($s$royal_league$s$, 3, $s$vanishadow$s$, $s$Inaugural Dynasty$s$,
     $s$Inaugural Back-to-Back Champion (Season 1 & 2)$s$,
     $s$vanishadow claimed the first two Royal League championships in history, capturing Gold in Season 1 and Season 2, followed by a Silver finish in Season 3.$s$),
    ($s$worlds$s$, 1, $s$a440$s$, $s$Reigning World Champion$s$,
     $s$The 2025 World Championship Title$s$,
     $s$a440 captured the 2025 Through the Ages World Championship, surviving the grueling multi-stage qualification and navigating the Stage 4 Upper playoff bracket to claim the world crown.$s$),
    ($s$worlds$s$, 2, $s$Martin_Pecheur$s$, $s$2024 World Champion$s$,
     $s$Playoff Sweep & World Championship Crown$s$,
     $s$Martin_Pecheur swept the 2024 World Championship playoff brackets, defeating top grandmasters in the championship finals.$s$),
    ($s$worlds$s$, 3, $s$Weidenbaum$s$, $s$Inaugural World Champion$s$,
     $s$The Inaugural 2023 World Championship$s$,
     $s$Weidenbaum was crowned the inaugural Through the Ages World Champion in 2023, adding the definitive global title to his record 6 International and 8 Intermezzo league championships.$s$);

-- +goose Down
REVOKE SELECT, INSERT, UPDATE, DELETE ON tournament_stories FROM ratings_admin;
DROP TABLE tournament_stories;
