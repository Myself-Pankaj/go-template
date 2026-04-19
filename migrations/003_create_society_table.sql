-- +migrate Up

-- ==================== SOCIETIES ====================

CREATE TABLE societies (
    id           BIGSERIAL    PRIMARY KEY,

    name         VARCHAR(150) NOT NULL,
    address      TEXT         NOT NULL,
    city         VARCHAR(100) NOT NULL,
    state        VARCHAR(100) NOT NULL,
    pin_code     VARCHAR(20)  NOT NULL,  -- column name matches Go model db:"pin_code"

    -- 10-char uppercase alphanumeric code generated at registration.
    -- Format: [5 prefix][4 pin digits][1 random] — see utils.GenerateSocietyCode.
    society_code VARCHAR(12)  NOT NULL,

    -- creator_id: the user who registered this society.
    -- Set inside the society-creation transaction (same tx as user.society_id assignment).
    -- DEFERRABLE INITIALLY DEFERRED allows the INSERT to resolve FK after user row exists
    -- within the same transaction without ordering concerns.
    creator_id   BIGINT       NOT NULL REFERENCES users(id) DEFERRABLE INITIALLY DEFERRED,

    -- is_active: controls login and feature access.
    -- Managed exclusively via Activate / Deactivate — never via the general Update endpoint.
    is_active    BOOLEAN      NOT NULL DEFAULT TRUE,

    -- deleted_at: soft-delete timestamp.
    -- NULL = not deleted. Non-null = deleted.
    -- All SELECT queries MUST include "WHERE deleted_at IS NULL" unless fetching audit data.
    -- DeleteSociety also sets is_active = FALSE so access blocks immediately.
    deleted_at   TIMESTAMP    NULL DEFAULT NULL,

    created_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- society_code is globally unique (no partial index needed — codes never change).
ALTER TABLE societies ADD CONSTRAINT uq_societies_society_code UNIQUE (society_code);

-- name+city uniqueness is enforced only among non-deleted rows.
-- Two societies with the same name+city can exist if one is soft-deleted (historical record).
CREATE UNIQUE INDEX uq_societies_name_city_active
    ON societies (name, city)
    WHERE deleted_at IS NULL;

-- FK: societies.creator_id → users.id (already declared inline above)
-- FK: users.society_id → societies.id (added below after societies table exists)
ALTER TABLE users
    ADD CONSTRAINT fk_users_society_id
    FOREIGN KEY (society_id) REFERENCES societies(id)
    DEFERRABLE INITIALLY DEFERRED;

-- Index: GetSocietyByID — the most common read path
CREATE INDEX idx_societies_id_active
    ON societies (id)
    WHERE deleted_at IS NULL;

-- Index: GetSocietyByCode — QR / onboarding lookups
CREATE INDEX idx_societies_code
    ON societies (society_code)
    WHERE deleted_at IS NULL;

-- Index: ListSocieties with ActiveOnly filter
CREATE INDEX idx_societies_is_active
    ON societies (is_active)
    WHERE deleted_at IS NULL;

-- Index: soft-delete guard — fast "deleted_at IS NULL" scans on large tables
CREATE INDEX idx_societies_deleted_at
    ON societies (deleted_at)
    WHERE deleted_at IS NULL;

-- Trigger: keep updated_at current on every UPDATE
-- update_timestamp() is defined in 001_create_users.sql
CREATE TRIGGER societies_update_timestamp
    BEFORE UPDATE ON societies
    FOR EACH ROW
    EXECUTE FUNCTION update_timestamp();


-- +migrate Down
ALTER TABLE   users          DROP CONSTRAINT IF EXISTS fk_users_society_id;
DROP TRIGGER  IF EXISTS      societies_update_timestamp ON societies;
DROP INDEX    IF EXISTS      idx_societies_deleted_at;
DROP INDEX    IF EXISTS      idx_societies_is_active;
DROP INDEX    IF EXISTS      idx_societies_code;
DROP INDEX    IF EXISTS      idx_societies_id_active;
DROP INDEX    IF EXISTS      uq_societies_name_city_active;
DROP TABLE    IF EXISTS      societies;