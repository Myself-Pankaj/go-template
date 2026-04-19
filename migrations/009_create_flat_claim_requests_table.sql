-- +migrate Up

-- Flat claim requests are created when a resident uses the General Society Join
-- QR code. They always require admin approval.
--
-- Statuses:
--   pending   submitted, awaiting admin review
--   approved  admin accepted → user.flat_id is set, flat moves to occupied
--   rejected  admin declined → rejection_reason is populated
CREATE TABLE flat_claim_requests (
    id               BIGSERIAL     PRIMARY KEY,

    user_id          BIGINT        NOT NULL,
    flat_id          BIGINT        NOT NULL,
    society_id       BIGINT        NOT NULL,

    status           VARCHAR(10)   NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending', 'approved', 'rejected')),

    -- Optional note from the resident (e.g. "I am the owner of flat A-101")
    note             TEXT          DEFAULT NULL,

    -- Admin who reviewed this request
    reviewed_by      BIGINT        DEFAULT NULL,
    reviewed_at      TIMESTAMP     DEFAULT NULL,

    -- Populated only on rejection
    rejection_reason TEXT          DEFAULT NULL,

    created_at       TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,

    CONSTRAINT fk_claim_user
        FOREIGN KEY (user_id)    REFERENCES users    (id) ON DELETE CASCADE,

    CONSTRAINT fk_claim_flat
        FOREIGN KEY (flat_id)    REFERENCES flats    (id) ON DELETE CASCADE,

    CONSTRAINT fk_claim_society
        FOREIGN KEY (society_id) REFERENCES societies(id) ON DELETE CASCADE,

    CONSTRAINT fk_claim_reviewer
        FOREIGN KEY (reviewed_by) REFERENCES users (id) ON DELETE SET NULL
);

-- Only one *pending* claim per (user, flat) at a time.
-- Allows re-submission after rejection on the same flat.
CREATE UNIQUE INDEX uq_pending_claim_user_flat
    ON flat_claim_requests (user_id, flat_id)
    WHERE status = 'pending';

-- Admin dashboard: list all pending claims in a society
CREATE INDEX idx_claim_society_status
    ON flat_claim_requests (society_id, status);

-- Lookup all claims for a specific flat
CREATE INDEX idx_claim_flat_id
    ON flat_claim_requests (flat_id);

-- Lookup all claims submitted by a user
CREATE INDEX idx_claim_user_id
    ON flat_claim_requests (user_id);

CREATE TRIGGER flat_claim_requests_update_timestamp
    BEFORE UPDATE ON flat_claim_requests
    FOR EACH ROW
    EXECUTE FUNCTION update_timestamp();

-- +migrate Down
DROP TRIGGER IF EXISTS flat_claim_requests_update_timestamp ON flat_claim_requests;
DROP TABLE IF EXISTS flat_claim_requests;
