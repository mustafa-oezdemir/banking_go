DROP TRIGGER IF EXISTS admin_audit_events_append_only ON admin_audit_events;
DROP FUNCTION IF EXISTS reject_admin_audit_event_mutation();
DROP TABLE IF EXISTS admin_audit_events;
ALTER TABLE users DROP COLUMN IF EXISTS session_version;
