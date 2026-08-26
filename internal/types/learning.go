package types

import "time"

const (
	LearningEvidenceDisplayedReference = "displayed_reference"
	LearningEvidenceQuizAttempt        = "quiz_attempt"

	LearningProfileStatusUnseen         = "unseen"
	LearningProfileStatusExposed        = "exposed"
	LearningProfileStatusUncertain      = "uncertain"
	LearningProfileStatusVerifiedWeak   = "verified_weak"
	LearningProfileStatusVerifiedStrong = "verified_strong"

	LearningConceptIdentityActive   = "active"
	LearningConceptIdentityOrphaned = "orphaned"

	LearningScanStatusPending   = "pending"
	LearningScanStatusActive    = "active"
	LearningScanStatusCompleted = "completed"
	LearningScanStatusCancelled = "cancelled"
)

// LearningProfile owns the privacy lifecycle for one caller inside one KB.
// It deliberately survives Clear so late asynchronous events can be rejected
// against ClearedAt even after all subject data has been deleted.
type LearningProfile struct {
	ID              string     `json:"id" gorm:"primaryKey;type:varchar(36)"`
	TenantID        uint64     `json:"tenant_id" gorm:"not null;uniqueIndex:idx_learning_profiles_scope,priority:1"`
	SubjectID       string     `json:"subject_id" gorm:"type:varchar(512);not null;uniqueIndex:idx_learning_profiles_scope,priority:2"`
	KnowledgeBaseID string     `json:"knowledge_base_id" gorm:"type:varchar(36);not null;uniqueIndex:idx_learning_profiles_scope,priority:3"`
	TrackingEnabled bool       `json:"tracking_enabled" gorm:"not null;default:false"`
	EnabledAt       *time.Time `json:"enabled_at,omitempty"`
	ClearedAt       *time.Time `json:"cleared_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (LearningProfile) TableName() string { return "learning_profiles" }

// LearningConceptIdentity is the stable identity of a Wiki concept. Wiki page
// UUIDs and slugs may change during rename/rebuild; ConceptKey does not.
type LearningConceptIdentity struct {
	ConceptKey        string      `json:"concept_key" gorm:"primaryKey;type:varchar(36)"`
	TenantID          uint64      `json:"tenant_id" gorm:"not null;index:idx_learning_concepts_scope"`
	KnowledgeBaseID   string      `json:"knowledge_base_id" gorm:"type:varchar(36);not null;index:idx_learning_concepts_scope"`
	CurrentWikiPageID *string     `json:"current_wiki_page_id,omitempty" gorm:"type:varchar(36)"`
	Slug              string      `json:"slug" gorm:"type:varchar(255);not null;default:''"`
	NormalizedSlug    string      `json:"normalized_slug" gorm:"type:varchar(255);not null;default:'';index"`
	Title             string      `json:"title" gorm:"type:varchar(512);not null;default:''"`
	NormalizedTitle   string      `json:"normalized_title" gorm:"type:varchar(512);not null;default:'';index"`
	Aliases           StringArray `json:"aliases" gorm:"type:json"`
	LearningEligible  bool        `json:"is_learning_eligible" gorm:"column:is_learning_eligible;not null;default:true"`
	Status            string      `json:"status" gorm:"type:varchar(16);not null;default:'active'"`
	CreatedAt         time.Time   `json:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at"`
}

func (LearningConceptIdentity) TableName() string { return "learning_concept_identities" }

// LearningEvidence is append-only. Its semantic writers arrive in later
// phases; Phase 1 establishes its scope, privacy and idempotency guarantees.
type LearningEvidence struct {
	ID              string    `json:"id" gorm:"primaryKey;type:varchar(36)"`
	TenantID        uint64    `json:"tenant_id" gorm:"not null;index:idx_learning_evidence_scope"`
	SubjectID       string    `json:"subject_id" gorm:"type:varchar(512);not null;index:idx_learning_evidence_scope"`
	KnowledgeBaseID string    `json:"knowledge_base_id" gorm:"type:varchar(36);not null;index:idx_learning_evidence_scope"`
	ConceptKey      string    `json:"concept_key" gorm:"type:varchar(36);not null;index"`
	WikiPageID      *string   `json:"wiki_page_id,omitempty" gorm:"type:varchar(36)"`
	SessionID       *string   `json:"session_id,omitempty" gorm:"type:varchar(36)"`
	MessageID       *string   `json:"message_id,omitempty" gorm:"type:varchar(36)"`
	KnowledgeID     *string   `json:"knowledge_id,omitempty" gorm:"type:varchar(36)"`
	ChunkID         *string   `json:"chunk_id,omitempty" gorm:"type:varchar(36)"`
	QuizItemID      *string   `json:"quiz_item_id,omitempty" gorm:"type:varchar(36)"`
	QuizAttemptID   *string   `json:"quiz_attempt_id,omitempty" gorm:"type:varchar(36)"`
	EventType       string    `json:"event_type" gorm:"type:varchar(64);not null"`
	EvidenceValue   float64   `json:"evidence_value" gorm:"not null;default:0"`
	Confidence      float64   `json:"confidence" gorm:"not null;default:0"`
	Metadata        JSONMap   `json:"metadata,omitempty" gorm:"type:json"`
	OccurredAt      time.Time `json:"occurred_at" gorm:"not null;index"`
	IdempotencyKey  string    `json:"idempotency_key" gorm:"type:varchar(255);not null;uniqueIndex"`
	CreatedAt       time.Time `json:"created_at"`
}

func (LearningEvidence) TableName() string { return "learning_evidence" }

type UserConceptState struct {
	ID                string     `json:"id" gorm:"primaryKey;type:varchar(36)"`
	TenantID          uint64     `json:"tenant_id" gorm:"not null;uniqueIndex:idx_user_concept_states_scope,priority:1"`
	SubjectID         string     `json:"subject_id" gorm:"type:varchar(512);not null;uniqueIndex:idx_user_concept_states_scope,priority:2"`
	KnowledgeBaseID   string     `json:"knowledge_base_id" gorm:"type:varchar(36);not null;uniqueIndex:idx_user_concept_states_scope,priority:3"`
	ConceptKey        string     `json:"concept_key" gorm:"type:varchar(36);not null;uniqueIndex:idx_user_concept_states_scope,priority:4"`
	ExposureCount     int        `json:"exposure_count" gorm:"not null;default:0"`
	ExposureWeight    float64    `json:"exposure_weight" gorm:"not null;default:0"`
	LastExposedAt     *time.Time `json:"last_exposed_at,omitempty"`
	VerifiedMastery   float64    `json:"verified_mastery" gorm:"not null;default:0"`
	MasteryConfidence float64    `json:"mastery_confidence" gorm:"not null;default:0"`
	QuizAttemptCount  int        `json:"quiz_attempt_count" gorm:"not null;default:0"`
	LastAssessedAt    *time.Time `json:"last_assessed_at,omitempty"`
	Status            string     `json:"status" gorm:"type:varchar(32);not null;default:'unseen'"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (UserConceptState) TableName() string { return "user_concept_states" }

type QuizItem struct {
	ID              string      `json:"id" gorm:"primaryKey;type:varchar(36)"`
	TenantID        uint64      `json:"tenant_id" gorm:"not null;index:idx_quiz_items_scope;uniqueIndex:idx_quiz_items_question_hash,priority:1"`
	KnowledgeBaseID string      `json:"knowledge_base_id" gorm:"type:varchar(36);not null;index:idx_quiz_items_scope;uniqueIndex:idx_quiz_items_question_hash,priority:2"`
	ConceptKey      string      `json:"concept_key" gorm:"type:varchar(36);not null;index:idx_quiz_items_scope;uniqueIndex:idx_quiz_items_question_hash,priority:3"`
	WikiPageID      *string     `json:"wiki_page_id,omitempty" gorm:"type:varchar(36)"`
	Question        string      `json:"question" gorm:"type:text;not null"`
	QuestionHash    string      `json:"-" gorm:"type:varchar(64);not null;default:'';uniqueIndex:idx_quiz_items_question_hash,priority:4"`
	Options         StringArray `json:"options" gorm:"type:json"`
	CorrectOption   int         `json:"-" gorm:"not null"`
	Explanation     string      `json:"explanation" gorm:"type:text"`
	SourceChunkIDs  StringArray `json:"source_chunk_ids" gorm:"type:json"`
	SourceHash      string      `json:"source_hash" gorm:"type:varchar(64);not null;index"`
	PromptVersion   string      `json:"prompt_version" gorm:"type:varchar(64);not null;default:''"`
	ModelInfo       JSONMap     `json:"model_info,omitempty" gorm:"type:json"`
	Difficulty      string      `json:"difficulty" gorm:"type:varchar(32);not null;default:''"`
	IsActive        bool        `json:"is_active" gorm:"not null;default:true"`
	IsStale         bool        `json:"is_stale" gorm:"not null;default:false;index"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
}

func (QuizItem) TableName() string { return "quiz_items" }

// QuizItemView is safe to return before a user answers a question. CorrectOption
// and Explanation deliberately remain server-side until the assessment phase.
type QuizItemView struct {
	ID             string      `json:"id"`
	ConceptKey     string      `json:"concept_key"`
	WikiPageID     *string     `json:"wiki_page_id,omitempty"`
	Question       string      `json:"question"`
	Options        StringArray `json:"options"`
	SourceChunkIDs StringArray `json:"source_chunk_ids"`
	SourceHash     string      `json:"source_hash"`
	PromptVersion  string      `json:"prompt_version"`
	Difficulty     string      `json:"difficulty"`
	CreatedAt      time.Time   `json:"created_at"`
}

type QuizBankResult struct {
	Status       string         `json:"status"`
	Items        []QuizItemView `json:"items"`
	Generated    int            `json:"generated"`
	AttemptCount int            `json:"attempt_count"`
	SourceHash   string         `json:"source_hash,omitempty"`
}

// LearningOverlay is the caller-scoped projection merged onto the shared Wiki
// graph. One response covers the whole KB; clients must not fetch per node.
type LearningOverlay struct {
	TrackingEnabled bool                  `json:"tracking_enabled"`
	Items           []LearningOverlayItem `json:"items"`
}

type LearningOverlayItem struct {
	ConceptKey        string     `json:"concept_key"`
	WikiPageID        string     `json:"wiki_page_id"`
	Slug              string     `json:"slug"`
	Status            string     `json:"status"`
	ExposureCount     int        `json:"exposure_count"`
	ExposureWeight    float64    `json:"exposure_weight"`
	VerifiedMastery   float64    `json:"verified_mastery"`
	MasteryConfidence float64    `json:"mastery_confidence"`
	QuizAttemptCount  int        `json:"quiz_attempt_count"`
	LastExposedAt     *time.Time `json:"last_exposed_at,omitempty"`
	LastAssessedAt    *time.Time `json:"last_assessed_at,omitempty"`
}

type LearningEvidenceView struct {
	ID            string    `json:"id"`
	EventType     string    `json:"event_type"`
	Confidence    float64   `json:"confidence"`
	EvidenceValue float64   `json:"evidence_value"`
	OccurredAt    time.Time `json:"occurred_at"`
	SessionID     *string   `json:"session_id,omitempty"`
	MessageID     *string   `json:"message_id,omitempty"`
	ChunkID       *string   `json:"chunk_id,omitempty"`
	QuizItemID    *string   `json:"quiz_item_id,omitempty"`
	QuizAttemptID *string   `json:"quiz_attempt_id,omitempty"`
}

type LearningRecommendation struct {
	ConceptKey string `json:"concept_key"`
	Title      string `json:"title"`
	Slug       string `json:"slug"`
	Reason     string `json:"reason"`
}

type LearningConceptInsights struct {
	ConceptKey     string                  `json:"concept_key"`
	Status         string                  `json:"status"`
	IsKnowledgeGap bool                    `json:"is_knowledge_gap"`
	GapReason      string                  `json:"gap_reason,omitempty"`
	Evidence       []LearningEvidenceView  `json:"evidence"`
	Recommendation *LearningRecommendation `json:"recommendation,omitempty"`
}

type QuizAttempt struct {
	ID              string    `json:"id" gorm:"primaryKey;type:varchar(36)"`
	TenantID        uint64    `json:"tenant_id" gorm:"not null;index:idx_quiz_attempts_scope"`
	SubjectID       string    `json:"subject_id" gorm:"type:varchar(512);not null;index:idx_quiz_attempts_scope"`
	KnowledgeBaseID string    `json:"knowledge_base_id" gorm:"type:varchar(36);not null;index:idx_quiz_attempts_scope"`
	QuizItemID      string    `json:"quiz_item_id" gorm:"type:varchar(36);not null;index"`
	ScanID          string    `json:"scan_id" gorm:"type:varchar(36);not null;index"`
	ConceptKey      string    `json:"concept_key" gorm:"type:varchar(36);not null;index"`
	SelectedOption  int       `json:"selected_option" gorm:"not null"`
	IsCorrect       bool      `json:"is_correct" gorm:"not null"`
	Score           float64   `json:"score" gorm:"not null;default:0"`
	AnsweredAt      time.Time `json:"answered_at" gorm:"not null"`
	IdempotencyKey  string    `json:"idempotency_key" gorm:"type:varchar(255);not null;uniqueIndex"`
	CreatedAt       time.Time `json:"created_at"`
}

func (QuizAttempt) TableName() string { return "quiz_attempts" }

// QuizAttemptRequest contains only caller-controlled answer data. Tenant and
// subject ownership are always derived from the authenticated principal.
type QuizAttemptRequest struct {
	ScanID         string `json:"scan_id"`
	SelectedOption int    `json:"selected_option"`
	IdempotencyKey string `json:"idempotency_key"`
}

// MasteryStateSnapshot is the assessment-owned portion of a concept state.
// Exposure fields are deliberately excluded from the answer response.
type MasteryStateSnapshot struct {
	Status            string  `json:"status"`
	VerifiedMastery   float64 `json:"verified_mastery"`
	MasteryConfidence float64 `json:"mastery_confidence"`
	QuizAttemptCount  int     `json:"quiz_attempt_count"`
}

type QuizAttemptResult struct {
	AttemptID         string               `json:"attempt_id"`
	IsCorrect         bool                 `json:"is_correct"`
	Explanation       string               `json:"explanation"`
	PreviousState     MasteryStateSnapshot `json:"previous_state"`
	NewState          MasteryStateSnapshot `json:"new_state"`
	VerifiedMastery   float64              `json:"verified_mastery"`
	MasteryConfidence float64              `json:"mastery_confidence"`
	Idempotent        bool                 `json:"idempotent"`
}

type LearningScan struct {
	ID              string      `json:"id" gorm:"primaryKey;type:varchar(36)"`
	TenantID        uint64      `json:"tenant_id" gorm:"not null;index:idx_learning_scans_scope"`
	SubjectID       string      `json:"subject_id" gorm:"type:varchar(512);not null;index:idx_learning_scans_scope"`
	KnowledgeBaseID string      `json:"knowledge_base_id" gorm:"type:varchar(36);not null;index:idx_learning_scans_scope"`
	Status          string      `json:"status" gorm:"type:varchar(32);not null;default:'pending'"`
	QuizItemIDs     StringArray `json:"quiz_item_ids" gorm:"type:json"`
	CurrentIndex    int         `json:"current_index" gorm:"not null;default:0"`
	CreatedAt       time.Time   `json:"created_at"`
	UpdatedAt       time.Time   `json:"updated_at"`
	CompletedAt     *time.Time  `json:"completed_at,omitempty"`
}

func (LearningScan) TableName() string { return "learning_scans" }

// LearningScanItem is the safe, resumable question payload returned to a
// caller. It deliberately embeds QuizItemView, which never exposes answers.
type LearningScanItem struct {
	QuizItemView
	ConceptTitle string `json:"concept_title"`
}

type LearningScanSummary struct {
	VerifiedStrong int `json:"verified_strong"`
	VerifiedWeak   int `json:"verified_weak"`
	Uncertain      int `json:"uncertain"`
}

// LearningScanView contains all state needed to resume one caller's scan.
type LearningScanView struct {
	ID           string              `json:"id"`
	Status       string              `json:"status"`
	CurrentIndex int                 `json:"current_index"`
	TotalItems   int                 `json:"total_items"`
	Items        []LearningScanItem  `json:"items"`
	Summary      LearningScanSummary `json:"summary"`
	CreatedAt    time.Time           `json:"created_at"`
	CompletedAt  *time.Time          `json:"completed_at,omitempty"`
}
