-- +migrate Up

CREATE TABLE subscriptions (
    id          SERIAL PRIMARY KEY,

    society_id  BIGINT NOT NULL REFERENCES societies(id),
    plan_id     BIGINT NOT NULL REFERENCES plans(id),

    status      VARCHAR(20) NOT NULL DEFAULT 'active'
                    CHECK (status IN ('active', 'cancel_pending', 'cancelled', 'expired')),

    is_trial    BOOLEAN NOT NULL DEFAULT FALSE,

    start_date  TIMESTAMP NOT NULL,
    end_date    TIMESTAMP NOT NULL,

    cancelled_at TIMESTAMP,                          -- NULL until status reaches cancelled/cancel_pending

    -- Plan snapshot: locked at subscription time so billing history
    -- remains accurate even if the plan is later repriced or deleted.
    snapshot_price          DECIMAL(10,2) NOT NULL,
    snapshot_billing_cycle  VARCHAR(20)   NOT NULL
                                CHECK (snapshot_billing_cycle IN ('monthly', 'yearly')),
    snapshot_max_flats      INT,                     -- NULL = unlimited
    snapshot_max_staff      INT,
    snapshot_max_admins     INT,

    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Only one active/cancel_pending subscription per society at a time.
-- Historical (cancelled/expired) rows are retained for audit, so we
-- use a partial unique index rather than a plain UNIQUE constraint.
CREATE UNIQUE INDEX uq_subscriptions_active_per_society
    ON subscriptions (society_id)
    WHERE status IN ('active', 'cancel_pending');

-- Speeds up GetActiveBySocietyID and IsActive lookups.
CREATE INDEX idx_subscriptions_society_status
    ON subscriptions (society_id, status);

-- Speeds up expiry background jobs that scan for end_date < NOW().
CREATE INDEX idx_subscriptions_end_date
    ON subscriptions (end_date)
    WHERE status IN ('active', 'cancel_pending');

CREATE TRIGGER subscriptions_update_timestamp
    BEFORE UPDATE ON subscriptions
    FOR EACH ROW
    EXECUTE FUNCTION update_timestamp();

-- +migrate Down

DROP TRIGGER IF EXISTS subscriptions_update_timestamp ON subscriptions;
DROP INDEX  IF EXISTS idx_subscriptions_end_date;
DROP INDEX  IF EXISTS idx_subscriptions_society_status;
DROP INDEX  IF EXISTS uq_subscriptions_active_per_society;
DROP TABLE  IF EXISTS subscriptions;