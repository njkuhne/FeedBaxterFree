CREATE UNIQUE INDEX tags_tenant_id_lower_name_idx ON tags (tenant_id, lower(name));
CREATE INDEX feedback_tenant_id_received_at_id_idx ON feedback (tenant_id, received_at DESC, id DESC);
CREATE INDEX personal_access_tokens_user_id_idx ON personal_access_tokens (user_id);
ALTER TABLE personal_access_tokens ADD COLUMN prefix TEXT NOT NULL DEFAULT '';
