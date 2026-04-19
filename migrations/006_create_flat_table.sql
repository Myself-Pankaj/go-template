-- +migrate Up
CREATE TABLE flats (
    id          BIGSERIAL    PRIMARY KEY,

    society_id  BIGINT       NOT NULL,
    flat_number VARCHAR(20)  NOT NULL,   -- e.g. "A-101", "B-202"
    floor       INT          DEFAULT NULL,
    block       VARCHAR(10)  DEFAULT NULL,

    -- Status values:
    --   active    flat is occupied / in use
    --   inactive  flat is vacant or temporarily disabled
    status      VARCHAR(10)  NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'inactive')),

    created_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- A flat number must be unique within a society
    CONSTRAINT uq_flats_society_flat UNIQUE (society_id, flat_number)
);

-- FK → societies
ALTER TABLE flats
    ADD CONSTRAINT fk_flats_society
    FOREIGN KEY (society_id) REFERENCES societies (id) ON DELETE CASCADE;

-- FK back-link: now that flats exists, wire users.flat_id
ALTER TABLE users
    ADD CONSTRAINT fk_users_flat
    FOREIGN KEY (flat_id) REFERENCES flats (id) ON DELETE SET NULL;

-- Index: list all flats in a society (admin dashboard)
CREATE INDEX idx_flats_society_id ON flats (society_id);

-- Trigger: keep updated_at current
CREATE TRIGGER flats_update_timestamp
    BEFORE UPDATE ON flats
    FOR EACH ROW
    EXECUTE FUNCTION update_timestamp();

-- +migrate Down
DROP TRIGGER IF EXISTS flats_update_timestamp ON flats;
ALTER TABLE users  DROP CONSTRAINT IF EXISTS fk_users_flat;
ALTER TABLE flats  DROP CONSTRAINT IF EXISTS fk_flats_society;
DROP TABLE IF EXISTS flats;