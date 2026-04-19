-- +migrate Up
CREATE TABLE users (
    id            BIGSERIAL    PRIMARY KEY,

    name          VARCHAR(100) NOT NULL,
    email         VARCHAR(255) NOT NULL,
    phone_number  VARCHAR(20)  NOT NULL,
    password_hash VARCHAR(255) NOT NULL,

    -- Role values:
    --   developer    full access for development/testing (not for production use)
    --   super_admin  assigned to person who will manager all societies and staff (initially the app owner)
    --   admin        society-level admin (can manage staff, residents)
    --   staff        security / maintenance personnel
    --   user         generic authenticated user (pre-society assignment)
    role          VARCHAR(20)  NOT NULL DEFAULT 'user'
                      CHECK (role IN ('developer', 'super_admin', 'admin', 'staff', 'user')),

    -- society_id is NULL until the user creates or is assigned to a society.
    -- Set atomically inside the society-creation transaction by UpdateSocietyID.
    society_id    BIGINT       DEFAULT NULL,

    -- flat_id is only applicable for role = 'user' (residents).
    -- NULL for all other roles; enforced by the chk_flat_id_only_for_user constraint.
    flat_id       BIGINT       DEFAULT NULL,

    is_verified   BOOLEAN      NOT NULL DEFAULT FALSE,
    last_login    TIMESTAMP    NULL,

    created_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP    NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- Ensure flat_id is only set for residents, never for staff/admins
    CONSTRAINT chk_flat_id_only_for_user
        CHECK (flat_id IS NULL OR role = 'user')
);

CREATE OR REPLACE FUNCTION update_timestamp()
RETURNS TRIGGER AS $$ BEGIN NEW.updated_at := CURRENT_TIMESTAMP; RETURN NEW; END; $$ LANGUAGE plpgsql;
-- Unique constraints
ALTER TABLE users ADD CONSTRAINT uq_users_email        UNIQUE (email);
ALTER TABLE users ADD CONSTRAINT uq_users_phone_number UNIQUE (phone_number);

-- Index: speeds up GetByEmail (login, registration duplicate check)
CREATE INDEX idx_users_email        ON users (LOWER(email));

-- Index: speeds up GetByPhoneNumber (login, registration duplicate check)
CREATE INDEX idx_users_phone_number ON users (phone_number);

-- Index: speeds up listing all users in a society (admin dashboard)
CREATE INDEX idx_users_society_id   ON users (society_id) WHERE society_id IS NOT NULL;

-- Index: speeds up listing all residents in a flat
CREATE INDEX idx_users_flat_id      ON users (flat_id) WHERE flat_id IS NOT NULL;

-- Trigger: keep updated_at current on every UPDATE
CREATE TRIGGER users_update_timestamp
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_timestamp();

-- +migrate Down
DROP TRIGGER IF EXISTS users_update_timestamp ON users;
DROP FUNCTION IF EXISTS update_timestamp();
DROP TABLE IF EXISTS users;