DROP INDEX IF EXISTS idx_quiz_items_question_hash;
ALTER TABLE quiz_items DROP COLUMN question_hash;
