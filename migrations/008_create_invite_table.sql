-- +migrate Up
CREATE TABLE flat_invites (
    id           BIGSERIAL     PRIMARY KEY,

    flat_id      BIGINT        NOT NULL,

    -- SHA-256 / CSPRNG token generated at app layer; stored as hex string
    token        VARCHAR(128)  NOT NULL,

    -- Who generated this invite (must be admin or super_admin)
    created_by   BIGINT        NOT NULL,

    -- How many times this token can be consumed (NULL = unlimited)
    max_uses     INT           DEFAULT 1,
    used_count   INT           NOT NULL DEFAULT 0,

    -- Hard expiry; app layer rejects tokens past this timestamp
    expires_at   TIMESTAMP     NOT NULL,

    -- Soft-delete / manual revocation by admin
    is_revoked   BOOLEAN       NOT NULL DEFAULT FALSE,

    created_at   TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT chk_invite_usage
        CHECK (max_uses IS NULL OR used_count <= max_uses)
);

ALTER TABLE flat_invites ADD CONSTRAINT uq_flat_invites_token    UNIQUE (token);

ALTER TABLE flat_invites
    ADD CONSTRAINT fk_flat_invites_flat
    FOREIGN KEY (flat_id) REFERENCES flats (id) ON DELETE CASCADE;

ALTER TABLE flat_invites
    ADD CONSTRAINT fk_flat_invites_created_by
    FOREIGN KEY (created_by) REFERENCES users (id) ON DELETE RESTRICT;

-- Token lookup on every redemption (hot path)
CREATE INDEX idx_flat_invites_token      ON flat_invites (token);

-- Active invites per flat (admin view)
CREATE INDEX idx_flat_invites_flat_id    ON flat_invites (flat_id)
    WHERE is_revoked = FALSE;

CREATE INDEX idx_flat_invites_created_by ON flat_invites (created_by);

CREATE TRIGGER flat_invites_update_timestamp
    BEFORE UPDATE ON flat_invites
    FOR EACH ROW
    EXECUTE FUNCTION update_timestamp();

-- +migrate Down
DROP TRIGGER IF EXISTS flat_invites_update_timestamp ON flat_invites;
DROP TABLE IF EXISTS flat_invites;