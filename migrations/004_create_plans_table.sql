-- +migrate Up
CREATE TABLE plans (
    id SERIAL PRIMARY KEY,

    name VARCHAR(50) NOT NULL UNIQUE,  -- Basic, Pro, Enterprise
    price DECIMAL(10,2) NOT NULL,

    billing_cycle VARCHAR(20) NOT NULL
        CHECK (billing_cycle IN ('monthly', 'yearly')),

    max_flats INT,              -- NULL = unlimited
    max_staff INT,              -- optional restriction
    max_admins INT,             -- optional restriction

    is_active BOOLEAN DEFAULT TRUE,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER plans_update_timestamp
    BEFORE UPDATE ON plans
    FOR EACH ROW
    EXECUTE FUNCTION update_timestamp();

-- Insert default plans if they don't already exist
INSERT INTO plans (name, price, billing_cycle, max_flats, max_staff, max_admins)
VALUES
    ('Trial', 0.00, 'monthly', 25, 1, 1),
    ('Basic', 799.00, 'monthly', 50, 3, 2),
    ('Premium', 2999.00, 'monthly', NULL, NULL, NULL)
ON CONFLICT (name) DO NOTHING;
-- +migrate Down
DROP TRIGGER IF EXISTS plans_update_timestamp ON plans;
DROP TABLE IF EXISTS plans;