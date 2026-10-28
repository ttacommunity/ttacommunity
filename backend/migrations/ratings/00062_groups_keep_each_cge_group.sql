-- Two CGE groups of the same name in one stage are two groups: the CGE group joins the groups' unique key.
-- +goose Up
ALTER TABLE groups DROP CONSTRAINT groups_stage_id_name_cge_tournament_cge_stage_number_key;
ALTER TABLE groups ADD CONSTRAINT groups_stage_name_cge_key
    UNIQUE NULLS NOT DISTINCT (stage_id, name, cge_tournament, cge_stage_number, cge_group);

-- +goose Down
-- Fails while a stage holds two CGE groups of the same name: merge them first.
ALTER TABLE groups DROP CONSTRAINT groups_stage_name_cge_key;
ALTER TABLE groups ADD CONSTRAINT groups_stage_id_name_cge_tournament_cge_stage_number_key
    UNIQUE NULLS NOT DISTINCT (stage_id, name, cge_tournament, cge_stage_number);
