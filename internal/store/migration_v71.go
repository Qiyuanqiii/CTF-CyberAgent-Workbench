package store

var externalSkillProjectionStatements = []string{
	`CREATE VIEW run_external_skill_projections AS
		SELECT 'external_skill_projection.v1' AS protocol_version,
			selection.run_id AS run_id,
			selection.mode_revision AS mode_revision,
			selection.surface AS surface,
			selection.profile AS profile,
			selection.token_budget AS token_budget,
			selection.token_upper_bound AS token_upper_bound,
			selection.item_count AS item_count,
			selection.operator_confirmed AS operator_confirmed,
			selection.context_delivery_authorized AS context_delivery_authorized,
			selection.tool_capability_grant AS tool_capability_grant,
			(SELECT COUNT(*) FROM root_external_skill_context_preparations preparation
				WHERE preparation.run_id = selection.run_id
					AND preparation.selection_id = selection.id) AS root_prepared_count,
			(SELECT COUNT(*) FROM root_external_skill_context_commits commit_record
				JOIN root_external_skill_context_preparations preparation
					ON preparation.id = commit_record.preparation_id
				WHERE preparation.run_id = selection.run_id
					AND preparation.selection_id = selection.id) AS root_committed_count,
			(SELECT COUNT(*) FROM specialist_external_skill_context_preparations preparation
				WHERE preparation.run_id = selection.run_id
					AND preparation.parent_selection_id = selection.id) AS specialist_prepared_count,
			(SELECT COUNT(*) FROM specialist_external_skill_context_commits commit_record
				JOIN specialist_external_skill_context_preparations preparation
					ON preparation.id = commit_record.preparation_id
				WHERE preparation.run_id = selection.run_id
					AND preparation.parent_selection_id = selection.id) AS specialist_committed_count,
			selection.created_at AS created_at
		FROM run_external_skill_selections selection;`,
	`CREATE VIEW run_external_skill_projection_items AS
		SELECT selection.run_id AS run_id,
			item.ordinal AS ordinal,
			item.name AS name,
			item.version AS version,
			item.token_upper_bound AS token_upper_bound,
			item.trust_class AS trust_class,
			item.tool_dependency_count AS declared_tool_count,
			item.specialist_eligible AS specialist_eligible
		FROM run_external_skill_selection_items item
		JOIN run_external_skill_selections selection ON selection.id = item.selection_id;`,
}
