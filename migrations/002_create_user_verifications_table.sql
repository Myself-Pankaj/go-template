-- +migrate Up
CREATE TABLE user_verifications (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    otp VARCHAR(6) NOT NULL,
    is_used BOOLEAN DEFAULT FALSE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_active_verification UNIQUE (user_id, is_used)
);


CREATE INDEX idx_user_verifications_user_id ON user_verifications(user_id);
CREATE INDEX idx_user_verifications_expires_at ON user_verifications(expires_at);

-- +migrate Down
DROP INDEX IF EXISTS idx_user_verifications_expires_at;
DROP INDEX IF EXISTS idx_user_verifications_user_id;
DROP TABLE IF EXISTS user_verifications;
