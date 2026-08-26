-- Knowledge MRI data foundation. Learning data is independent from Memory and
-- every subject-owned row is scoped by tenant + principal storage id + KB.

CREATE TABLE IF NOT EXISTS learning_profiles (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    subject_id VARCHAR(512) NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    tracking_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    enabled_at TIMESTAMP WITH TIME ZONE,
    cleared_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_learning_profiles_scope
    ON learning_profiles (tenant_id, subject_id, knowledge_base_id);

CREATE TABLE IF NOT EXISTS learning_concept_identities (
    concept_key VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    current_wiki_page_id VARCHAR(36),
    slug VARCHAR(255) NOT NULL DEFAULT '',
    normalized_slug VARCHAR(255) NOT NULL DEFAULT '',
    title VARCHAR(512) NOT NULL DEFAULT '',
    normalized_title VARCHAR(512) NOT NULL DEFAULT '',
    aliases JSONB NOT NULL DEFAULT '[]'::JSONB,
    is_learning_eligible BOOLEAN NOT NULL DEFAULT TRUE,
    status VARCHAR(16) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_learning_concepts_scope
    ON learning_concept_identities (tenant_id, knowledge_base_id);
CREATE INDEX IF NOT EXISTS idx_learning_concepts_slug
    ON learning_concept_identities (tenant_id, knowledge_base_id, normalized_slug);
CREATE INDEX IF NOT EXISTS idx_learning_concepts_title
    ON learning_concept_identities (tenant_id, knowledge_base_id, normalized_title);
CREATE UNIQUE INDEX IF NOT EXISTS idx_learning_concepts_page
    ON learning_concept_identities (tenant_id, knowledge_base_id, current_wiki_page_id)
    WHERE current_wiki_page_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS user_concept_states (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    subject_id VARCHAR(512) NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    concept_key VARCHAR(36) NOT NULL,
    exposure_count INTEGER NOT NULL DEFAULT 0,
    exposure_weight DOUBLE PRECISION NOT NULL DEFAULT 0,
    last_exposed_at TIMESTAMP WITH TIME ZONE,
    verified_mastery DOUBLE PRECISION NOT NULL DEFAULT 0,
    mastery_confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    quiz_attempt_count INTEGER NOT NULL DEFAULT 0,
    last_assessed_at TIMESTAMP WITH TIME ZONE,
    status VARCHAR(32) NOT NULL DEFAULT 'unseen',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_concept_states_scope
    ON user_concept_states (tenant_id, subject_id, knowledge_base_id, concept_key);

CREATE TABLE IF NOT EXISTS quiz_items (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    concept_key VARCHAR(36) NOT NULL,
    wiki_page_id VARCHAR(36),
    question TEXT NOT NULL,
    options JSONB NOT NULL DEFAULT '[]'::JSONB,
    correct_option INTEGER NOT NULL,
    explanation TEXT NOT NULL DEFAULT '',
    source_chunk_ids JSONB NOT NULL DEFAULT '[]'::JSONB,
    source_hash VARCHAR(64) NOT NULL,
    prompt_version VARCHAR(64) NOT NULL DEFAULT '',
    model_info JSONB,
    difficulty VARCHAR(32) NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    is_stale BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_quiz_items_scope
    ON quiz_items (tenant_id, knowledge_base_id, concept_key, is_active, is_stale);
CREATE INDEX IF NOT EXISTS idx_quiz_items_source_hash ON quiz_items (source_hash);

CREATE TABLE IF NOT EXISTS learning_scans (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    subject_id VARCHAR(512) NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    status VARCHAR(32) NOT NULL DEFAULT 'pending',
    quiz_item_ids JSONB NOT NULL DEFAULT '[]'::JSONB,
    current_index INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_learning_scans_scope
    ON learning_scans (tenant_id, subject_id, knowledge_base_id, created_at DESC);

CREATE TABLE IF NOT EXISTS quiz_attempts (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    subject_id VARCHAR(512) NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    quiz_item_id VARCHAR(36) NOT NULL,
    scan_id VARCHAR(36) NOT NULL,
    concept_key VARCHAR(36) NOT NULL,
    selected_option INTEGER NOT NULL,
    is_correct BOOLEAN NOT NULL,
    score DOUBLE PRECISION NOT NULL DEFAULT 0,
    answered_at TIMESTAMP WITH TIME ZONE NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_quiz_attempts_idempotency
    ON quiz_attempts (idempotency_key);
CREATE INDEX IF NOT EXISTS idx_quiz_attempts_scope
    ON quiz_attempts (tenant_id, subject_id, knowledge_base_id, concept_key, answered_at DESC);
CREATE INDEX IF NOT EXISTS idx_quiz_attempts_scan ON quiz_attempts (scan_id);
CREATE INDEX IF NOT EXISTS idx_quiz_attempts_item ON quiz_attempts (quiz_item_id);

CREATE TABLE IF NOT EXISTS learning_evidence (
    id VARCHAR(36) PRIMARY KEY,
    tenant_id BIGINT NOT NULL,
    subject_id VARCHAR(512) NOT NULL,
    knowledge_base_id VARCHAR(36) NOT NULL,
    concept_key VARCHAR(36) NOT NULL,
    wiki_page_id VARCHAR(36),
    session_id VARCHAR(36),
    message_id VARCHAR(36),
    knowledge_id VARCHAR(36),
    chunk_id VARCHAR(36),
    quiz_item_id VARCHAR(36),
    quiz_attempt_id VARCHAR(36),
    event_type VARCHAR(64) NOT NULL,
    evidence_value DOUBLE PRECISION NOT NULL DEFAULT 0,
    confidence DOUBLE PRECISION NOT NULL DEFAULT 0,
    metadata JSONB,
    occurred_at TIMESTAMP WITH TIME ZONE NOT NULL,
    idempotency_key VARCHAR(255) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_learning_evidence_idempotency
    ON learning_evidence (idempotency_key);
CREATE INDEX IF NOT EXISTS idx_learning_evidence_scope
    ON learning_evidence (tenant_id, subject_id, knowledge_base_id, concept_key, occurred_at DESC);
CREATE INDEX IF NOT EXISTS idx_learning_evidence_message ON learning_evidence (message_id);
CREATE INDEX IF NOT EXISTS idx_learning_evidence_chunk ON learning_evidence (chunk_id);
