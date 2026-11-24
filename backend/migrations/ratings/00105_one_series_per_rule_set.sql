-- One series per results rule set again: the engine reads the one series holding each set, so a second holder would be
-- silently ignored or take the first's place.
-- +goose Up
ALTER TABLE series ADD CONSTRAINT series_results_rules_key UNIQUE (results_rules);

-- +goose Down
ALTER TABLE series DROP CONSTRAINT series_results_rules_key;
