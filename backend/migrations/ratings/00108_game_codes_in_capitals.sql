-- +goose Up
-- +goose StatementBegin
-- games_code_in_capitals stores a game's share code in capitals, however it was typed or loaded.
CREATE FUNCTION games_code_in_capitals() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.share_code := upper(NEW.share_code);
    RETURN NEW;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER games_code_in_capitals BEFORE INSERT OR UPDATE OF share_code ON games
    FOR EACH ROW EXECUTE FUNCTION games_code_in_capitals();
UPDATE games SET share_code = share_code WHERE share_code <> upper(share_code);

-- +goose Down
-- Codes stay in capitals: which were typed otherwise is not kept.
DROP TRIGGER games_code_in_capitals ON games;
DROP FUNCTION games_code_in_capitals();
