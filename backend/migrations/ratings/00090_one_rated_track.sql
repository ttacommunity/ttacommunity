-- A tournament or group is rated in the standard track or the SuperBlitz track, never both.
-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION ranking_series_one_track() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE tracks constant text[] := ARRAY['community', 'superblitz'];
BEGIN
    IF (SELECT slug FROM rankings WHERE ranking_id = NEW.ranking_id) <> ALL (tracks) THEN
        RETURN NEW;
    END IF;
    PERFORM 1 FROM series WHERE series_id = NEW.series_id FOR UPDATE;
    IF EXISTS (SELECT 1 FROM ranking_series rs JOIN rankings r USING (ranking_id)
               WHERE rs.series_id = NEW.series_id AND rs.ranking_id <> NEW.ranking_id AND r.slug = ANY (tracks)) THEN
        RAISE EXCEPTION 'series % is rated in the standard or the SuperBlitz track, not both', NEW.series_id
            USING ERRCODE = 'check_violation', CONSTRAINT = 'ranking_series_one_track';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER ranking_series_one_track BEFORE INSERT OR UPDATE ON ranking_series
    FOR EACH ROW EXECUTE FUNCTION ranking_series_one_track();

-- +goose Down
DROP TRIGGER ranking_series_one_track ON ranking_series;
DROP FUNCTION ranking_series_one_track();
