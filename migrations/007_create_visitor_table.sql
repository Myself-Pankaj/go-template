-- +migrate Up
CREATE TABLE visitor_entries (
    id                BIGSERIAL     PRIMARY KEY,

    -- Visitors are linked to flats, not individual residents
    flat_id           BIGINT        NOT NULL,

    -- Basic visitor info captured at gate
    visitor_name      VARCHAR(100)  NOT NULL,
    visitor_phone     VARCHAR(20)   DEFAULT NULL,
    visitor_photo_url TEXT          DEFAULT NULL,  -- S3 / CDN URL set by gate staff
    purpose           VARCHAR(255)  DEFAULT NULL,  -- e.g. "Delivery", "Guest", "Service"

    -- The guard / staff member who logged this entry
    logged_by         BIGINT        NOT NULL,

    -- Lifecycle: PENDING → APPROVED / REJECTED
    --   PENDING   entry created by guard, awaiting resident approval
    --   APPROVED  resident approved; visitor allowed in
    --   REJECTED  resident rejected; visitor turned away
    status            VARCHAR(10)   NOT NULL DEFAULT 'PENDING'
                          CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED')),

    -- Who approved or rejected (any user belonging to the flat)
    actioned_by       BIGINT        DEFAULT NULL,
    actioned_at       TIMESTAMP     DEFAULT NULL,

    -- Timestamps for physical entry and exit at gate
    entry_time        TIMESTAMP     DEFAULT NULL,  -- set when visitor physically enters
    expected_exit_time TIMESTAMP    DEFAULT NULL,  -- optional, set at entry (MVP)
    exit_time         TIMESTAMP     DEFAULT NULL,  -- set when visitor physically exits

    created_at        TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMP     NOT NULL DEFAULT CURRENT_TIMESTAMP,

    -- exit cannot be before entry
    CONSTRAINT chk_exit_after_entry
        CHECK (exit_time IS NULL OR entry_time IS NULL OR exit_time >= entry_time),

    -- actioned_by and actioned_at must be set together
    CONSTRAINT chk_action_consistency
        CHECK (
            (actioned_by IS NULL AND actioned_at IS NULL) OR
            (actioned_by IS NOT NULL AND actioned_at IS NOT NULL)
        )
);

-- FK → flats
ALTER TABLE visitor_entries
    ADD CONSTRAINT fk_visitor_entries_flat
    FOREIGN KEY (flat_id) REFERENCES flats (id) ON DELETE RESTRICT;

-- FK → users (guard who created the entry)
ALTER TABLE visitor_entries
    ADD CONSTRAINT fk_visitor_entries_logged_by
    FOREIGN KEY (logged_by) REFERENCES users (id) ON DELETE RESTRICT;

-- FK → users (resident who approved/rejected)
ALTER TABLE visitor_entries
    ADD CONSTRAINT fk_visitor_entries_actioned_by
    FOREIGN KEY (actioned_by) REFERENCES users (id) ON DELETE RESTRICT;

-- Hot path: all visitors for a flat (resident dashboard)
CREATE INDEX idx_visitor_entries_flat_id
    ON visitor_entries (flat_id);

-- Filter active/pending visitors quickly
CREATE INDEX idx_visitor_entries_status
    ON visitor_entries (status)
    WHERE status = 'PENDING';

-- Gate staff query: currently inside visitors (entered but not exited)
CREATE INDEX idx_visitor_entries_active
    ON visitor_entries (flat_id, entry_time)
    WHERE entry_time IS NOT NULL AND exit_time IS NULL;

-- Audit: entries logged by a specific guard
CREATE INDEX idx_visitor_entries_logged_by
    ON visitor_entries (logged_by);

CREATE TRIGGER visitor_entries_update_timestamp
    BEFORE UPDATE ON visitor_entries
    FOR EACH ROW
    EXECUTE FUNCTION update_timestamp();

-- +migrate Down
DROP TRIGGER IF EXISTS visitor_entries_update_timestamp ON visitor_entries;
DROP TABLE IF EXISTS visitor_entries;

