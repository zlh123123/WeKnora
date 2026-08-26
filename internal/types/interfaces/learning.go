package interfaces

import (
	"context"
	"errors"

	"github.com/Tencent/WeKnora/internal/types"
)

var (
	ErrLearningTrackingDisabled = errors.New("learning: tracking is disabled")
	ErrQuizItemUnavailable      = errors.New("learning: quiz item is unavailable")
	ErrLearningScanUnavailable  = errors.New("learning: scan is unavailable")
	ErrQuizItemNotInScan        = errors.New("learning: quiz item is not in scan")
	ErrInvalidQuizAttempt       = errors.New("learning: invalid quiz attempt")
)

type LearningScope struct {
	TenantID        uint64
	SubjectID       string
	KnowledgeBaseID string
}

func (s LearningScope) Valid() bool {
	return s.TenantID > 0 && s.SubjectID != "" && s.KnowledgeBaseID != ""
}

type LearningConceptLookup struct {
	CurrentWikiPageID string
	Slug              string
	Title             string
	Aliases           []string
}

type LearningConceptResolution struct {
	Identity  *types.LearningConceptIdentity
	MatchedBy string
	Ambiguous bool
}

const (
	LearningConceptMatchedPageID     = "page_id"
	LearningConceptMatchedSlug       = "slug"
	LearningConceptMatchedAlias      = "alias"
	LearningConceptMatchedTitle      = "title"
	LearningConceptMatchedUnresolved = "unresolved"
	LearningConceptMatchedAmbiguous  = "ambiguous"
)

// LearningRepository keeps all subject-owned operations explicitly scoped.
// Callers cannot select another subject through a request payload.
type LearningRepository interface {
	GetProfile(ctx context.Context, scope LearningScope) (*types.LearningProfile, error)
	EnsureProfile(ctx context.Context, scope LearningScope) (*types.LearningProfile, error)
	SetTracking(ctx context.Context, scope LearningScope, enabled bool) (*types.LearningProfile, error)
	ClearScope(ctx context.Context, scope LearningScope) (*types.LearningProfile, error)

	CreateConceptIdentity(ctx context.Context, identity *types.LearningConceptIdentity) error
	UpdateConceptIdentity(ctx context.Context, identity *types.LearningConceptIdentity) error
	ResolveConceptIdentity(
		ctx context.Context, tenantID uint64, knowledgeBaseID string, lookup LearningConceptLookup,
	) (LearningConceptResolution, error)
	GetConceptIdentity(ctx context.Context, tenantID uint64, knowledgeBaseID, conceptKey string) (*types.LearningConceptIdentity, error)
	ListConceptIdentities(ctx context.Context, tenantID uint64, knowledgeBaseID string) ([]*types.LearningConceptIdentity, error)

	ListQuizItems(ctx context.Context, tenantID uint64, knowledgeBaseID, conceptKey, sourceHash string) ([]*types.QuizItem, error)
	SaveQuizItems(ctx context.Context, tenantID uint64, knowledgeBaseID, conceptKey, sourceHash string, items []*types.QuizItem) ([]*types.QuizItem, error)
	SubmitQuizAttempt(ctx context.Context, scope LearningScope, itemID string, request types.QuizAttemptRequest) (*types.QuizAttemptResult, error)
	GetActiveScan(ctx context.Context, scope LearningScope) (*types.LearningScan, error)
	CreateScan(ctx context.Context, scope LearningScope, scan *types.LearningScan) error
	GetScan(ctx context.Context, scope LearningScope, scanID string) (*types.LearningScan, error)
	ListQuizAttemptsForScan(ctx context.Context, scope LearningScope, scanID string) ([]*types.QuizAttempt, error)
	ListQuizAttempts(ctx context.Context, scope LearningScope) ([]*types.QuizAttempt, error)
	CompleteScan(ctx context.Context, scope LearningScope, scanID string) (*types.LearningScan, error)

	GetConceptState(ctx context.Context, scope LearningScope, conceptKey string) (*types.UserConceptState, error)
	ListConceptStates(ctx context.Context, scope LearningScope) ([]*types.UserConceptState, error)

	AppendEvidence(ctx context.Context, scope LearningScope, evidence *types.LearningEvidence) (bool, error)
	AppendExposureEvidence(ctx context.Context, scope LearningScope, evidence *types.LearningEvidence) (bool, error)
	ListEvidence(ctx context.Context, scope LearningScope, conceptKey string, limit int) ([]*types.LearningEvidence, error)
	CountEvidence(ctx context.Context, scope LearningScope) (int64, error)
}

type LearningService interface {
	GetProfile(ctx context.Context, knowledgeBaseID string) (*types.LearningProfile, error)
	SetTracking(ctx context.Context, knowledgeBaseID string, enabled bool) (*types.LearningProfile, error)
	Clear(ctx context.Context, knowledgeBaseID string) (*types.LearningProfile, error)
	EnsureConceptIdentity(
		ctx context.Context, knowledgeBaseID string, lookup LearningConceptLookup,
	) (*types.LearningConceptIdentity, LearningConceptResolution, error)
	RecordDisplayedReferences(ctx context.Context, message *types.Message) error
	ListConceptStates(ctx context.Context, knowledgeBaseID string) ([]*types.UserConceptState, error)
	GetQuizItems(ctx context.Context, knowledgeBaseID, conceptKey string) (*types.QuizBankResult, error)
	GenerateQuizItems(ctx context.Context, knowledgeBaseID, conceptKey string) (*types.QuizBankResult, error)
	SubmitQuizAttempt(ctx context.Context, knowledgeBaseID, itemID string, request types.QuizAttemptRequest) (*types.QuizAttemptResult, error)
	StartOrResumeScan(ctx context.Context, knowledgeBaseID string) (*types.LearningScanView, error)
	GetActiveScan(ctx context.Context, knowledgeBaseID string) (*types.LearningScanView, error)
	CompleteScan(ctx context.Context, knowledgeBaseID, scanID string) (*types.LearningScanView, error)
	GetLearningOverlay(ctx context.Context, knowledgeBaseID string) (*types.LearningOverlay, error)
	GetConceptInsights(ctx context.Context, knowledgeBaseID, conceptKey string) (*types.LearningConceptInsights, error)
}
