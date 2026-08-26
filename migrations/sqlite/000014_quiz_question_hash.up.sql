ALTER TABLE quiz_items
    ADD COLUMN question_hash VARCHAR(64) NOT NULL DEFAULT '';

CREATE UNIQUE INDEX IF NOT EXISTS idx_quiz_items_question_hash
    ON quiz_items (tenant_id, knowledge_base_id, concept_key, question_hash);
