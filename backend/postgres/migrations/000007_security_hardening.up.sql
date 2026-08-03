ALTER TABLE users
    ADD COLUMN session_version BIGINT NOT NULL DEFAULT 0;

CREATE TABLE admin_audit_events (
    id BIGSERIAL PRIMARY KEY,
    actor_user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    target_user_id UUID REFERENCES users(id) ON DELETE RESTRICT,
    target_account_id UUID REFERENCES accounts(id) ON DELETE RESTRICT,
    action VARCHAR(64) NOT NULL,
    before_data JSONB NOT NULL DEFAULT '{}'::JSONB,
    after_data JSONB NOT NULL DEFAULT '{}'::JSONB,
    request_id VARCHAR(128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_admin_audit_events_actor_created
    ON admin_audit_events (actor_user_id, created_at DESC);
CREATE INDEX idx_admin_audit_events_target_user
    ON admin_audit_events (target_user_id, created_at DESC)
    WHERE target_user_id IS NOT NULL;
CREATE INDEX idx_admin_audit_events_target_account
    ON admin_audit_events (target_account_id, created_at DESC)
    WHERE target_account_id IS NOT NULL;

CREATE FUNCTION reject_admin_audit_event_mutation()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'admin audit events are append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER admin_audit_events_append_only
BEFORE UPDATE OR DELETE ON admin_audit_events
FOR EACH ROW EXECUTE FUNCTION reject_admin_audit_event_mutation();
