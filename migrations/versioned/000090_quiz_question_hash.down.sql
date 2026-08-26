DROP INDEX IF EXISTS idx_quiz_items_question_hash;
ALTER TABLE quiz_items DROP COLUMN IF EXISTS question_hash;
