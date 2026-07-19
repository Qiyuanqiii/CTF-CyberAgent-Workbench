package store

var cumulativeHandoffMemoryStatements = []string{
	`ALTER TABLE context_summaries ADD COLUMN protocol_version TEXT NOT NULL DEFAULT 'handoff_memory.v0';`,
	`ALTER TABLE context_summaries ADD COLUMN previous_summary_id INTEGER REFERENCES context_summaries(id);`,
	`ALTER TABLE context_summaries ADD COLUMN content_sha256 TEXT NOT NULL DEFAULT '';`,
	`ALTER TABLE context_summaries ADD COLUMN compacted_message_count INTEGER NOT NULL DEFAULT 0;`,
	`CREATE UNIQUE INDEX idx_context_summaries_previous_v1
		ON context_summaries(previous_summary_id)
		WHERE protocol_version = 'handoff_memory.v1' AND previous_summary_id IS NOT NULL;`,
	`CREATE TRIGGER trg_context_summaries_v1_insert
		BEFORE INSERT ON context_summaries
		WHEN NEW.protocol_version != 'handoff_memory.v1'
			OR length(NEW.content_sha256) != 64
			OR NEW.content_sha256 != lower(NEW.content_sha256)
			OR NEW.content_sha256 GLOB '*[^0-9a-f]*'
			OR NEW.compacted_message_count < 1
			OR NEW.source_message_count != NEW.compacted_message_count + NEW.preserved_message_count
			OR (NEW.previous_summary_id IS NULL AND EXISTS (
				SELECT 1 FROM context_summaries previous WHERE previous.task_id = NEW.task_id))
			OR (NEW.previous_summary_id IS NOT NULL AND NEW.previous_summary_id != (
				SELECT MAX(previous.id) FROM context_summaries previous WHERE previous.task_id = NEW.task_id))
			OR (NEW.previous_summary_id IS NOT NULL AND NOT EXISTS (
				SELECT 1 FROM context_summaries previous
				WHERE previous.id = NEW.previous_summary_id AND previous.task_id = NEW.task_id
					AND coalesce(previous.workspace_id, '') = coalesce(NEW.workspace_id, '')
					AND previous.created_at <= NEW.created_at
					AND (previous.protocol_version != 'handoff_memory.v1'
						OR previous.compacted_message_count < NEW.compacted_message_count)))
		BEGIN SELECT RAISE(ABORT, 'context handoff summary binding is invalid'); END;`,
	`CREATE TRIGGER trg_context_summaries_update_immutable
		BEFORE UPDATE ON context_summaries BEGIN
			SELECT RAISE(ABORT, 'context handoff summary cannot be updated');
		END;`,
	`CREATE TRIGGER trg_context_summaries_delete_immutable
		BEFORE DELETE ON context_summaries BEGIN
			SELECT RAISE(ABORT, 'context handoff summary cannot be deleted');
		END;`,
}
