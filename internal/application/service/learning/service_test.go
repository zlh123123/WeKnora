package learning

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type exposureWikiRepository struct {
	interfaces.WikiPageRepository
	pages map[string][]*types.WikiPage
}

func (r *exposureWikiRepository) ListByType(
	_ context.Context, kbID string, _ string,
) ([]*types.WikiPage, error) {
	return r.pages[kbID], nil
}

type exposureChunkRepository struct {
	interfaces.ChunkRepository
	chunks map[string]*types.Chunk
}

func (r *exposureChunkRepository) ListChunksByIDOnly(
	_ context.Context, ids []string,
) ([]*types.Chunk, error) {
	result := make([]*types.Chunk, 0, len(ids))
	for _, id := range ids {
		if chunk := r.chunks[id]; chunk != nil {
			result = append(result, chunk)
		}
	}
	return result, nil
}

type exposureKBService struct {
	interfaces.KnowledgeBaseService
	kbs map[string]*types.KnowledgeBase
}

func (s *exposureKBService) GetKnowledgeBaseByIDOnly(
	_ context.Context, id string,
) (*types.KnowledgeBase, error) {
	if kb := s.kbs[id]; kb != nil {
		return kb, nil
	}
	return nil, fmt.Errorf("knowledge base not found")
}

func newExposureServiceTest(
	t *testing.T,
) (interfaces.LearningService, interfaces.LearningRepository, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.LearningProfile{}, &types.LearningConceptIdentity{}, &types.UserConceptState{},
		&types.QuizItem{}, &types.LearningScan{}, &types.QuizAttempt{}, &types.LearningEvidence{},
	))
	repo := repository.NewLearningRepository(db)
	wikiRepo := &exposureWikiRepository{pages: map[string][]*types.WikiPage{
		"kb-shared": {
			{ID: "page-a", TenantID: 9, KnowledgeBaseID: "kb-shared", Slug: "concept/a", Title: "A", PageType: types.WikiPageTypeConcept, Status: types.WikiPageStatusPublished, ChunkRefs: types.StringArray{"chunk-single", "chunk-multi"}},
			{ID: "page-b", TenantID: 9, KnowledgeBaseID: "kb-shared", Slug: "concept/b", Title: "B", PageType: types.WikiPageTypeConcept, Status: types.WikiPageStatusPublished, ChunkRefs: types.StringArray{"chunk-multi"}},
		},
	}}
	chunkRepo := &exposureChunkRepository{chunks: map[string]*types.Chunk{
		"chunk-single": {ID: "chunk-single", TenantID: 9, KnowledgeBaseID: "kb-shared", KnowledgeID: "knowledge-1"},
		"chunk-multi":  {ID: "chunk-multi", TenantID: 9, KnowledgeBaseID: "kb-shared", KnowledgeID: "knowledge-1"},
	}}
	kbService := &exposureKBService{kbs: map[string]*types.KnowledgeBase{
		"kb-shared": {ID: "kb-shared", TenantID: 9},
	}}
	return NewService(repo, wikiRepo, chunkRepo, kbService, nil), repo, db
}

func displayedMessage(id string, occurredAt time.Time, chunkIDs ...string) *types.Message {
	refs := make(types.References, 0, len(chunkIDs))
	for _, chunkID := range chunkIDs {
		refs = append(refs, &types.SearchResult{
			ID: chunkID, KnowledgeID: "knowledge-1", KnowledgeBaseID: "kb-shared",
		})
	}
	return &types.Message{
		ID: id, SessionID: "session-1", Role: "assistant", IsCompleted: true,
		KnowledgeReferences: refs, UpdatedAt: occurredAt,
	}
}

func newLearningServiceTest(t *testing.T) interfaces.LearningService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.LearningProfile{}, &types.LearningConceptIdentity{}, &types.UserConceptState{},
		&types.QuizItem{}, &types.LearningScan{}, &types.QuizAttempt{}, &types.LearningEvidence{},
	))
	return NewService(repository.NewLearningRepository(db), nil, nil, nil, nil)
}

func learningWebContext(tenantID uint64, userID string) context.Context {
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, tenantID)
	return types.WithPrincipal(ctx, types.Principal{Type: types.PrincipalWebUser, ID: userID})
}

func TestLearningServiceDerivesSubjectFromWebPrincipal(t *testing.T) {
	svc := newLearningServiceTest(t)
	profile, err := svc.GetProfile(learningWebContext(7, "alice"), "kb-a")
	require.NoError(t, err)
	require.Equal(t, uint64(7), profile.TenantID)
	require.Equal(t, "web_user:alice", profile.SubjectID)
	require.Equal(t, "kb-a", profile.KnowledgeBaseID)
	require.False(t, profile.TrackingEnabled)
}

func TestLearningServiceRejectsNonWebPrincipal(t *testing.T) {
	svc := newLearningServiceTest(t)
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	ctx = types.WithPrincipal(ctx, types.Principal{Type: types.PrincipalIMUser, ID: "im-user"})
	_, err := svc.GetProfile(ctx, "kb-a")
	require.ErrorIs(t, err, ErrUnsupportedPrincipal)
}

func TestLearningServiceCreatesOrphanOnAmbiguousIdentity(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.LearningProfile{}, &types.LearningConceptIdentity{}, &types.UserConceptState{},
		&types.QuizItem{}, &types.LearningScan{}, &types.QuizAttempt{}, &types.LearningEvidence{},
	))
	repo := repository.NewLearningRepository(db)
	svc := NewService(repo, nil, nil, nil, nil)
	ctx := learningWebContext(1, "alice")
	for _, identity := range []*types.LearningConceptIdentity{
		{ConceptKey: "concept-a", TenantID: 1, KnowledgeBaseID: "kb-a", Slug: "concept/a", Title: "A", Aliases: types.StringArray{"shared"}, LearningEligible: true},
		{ConceptKey: "concept-b", TenantID: 1, KnowledgeBaseID: "kb-a", Slug: "concept/b", Title: "B", Aliases: types.StringArray{"shared"}, LearningEligible: true},
	} {
		require.NoError(t, repo.CreateConceptIdentity(ctx, identity))
	}

	identity, resolution, err := svc.EnsureConceptIdentity(ctx, "kb-a", interfaces.LearningConceptLookup{
		CurrentWikiPageID: "page-c", Slug: "concept/c", Title: "shared",
	})
	require.NoError(t, err)
	require.True(t, resolution.Ambiguous)
	require.Equal(t, types.LearningConceptIdentityOrphaned, identity.Status)
	require.NotEmpty(t, identity.ConceptKey)
}

func TestResolveLearningScopeRequiresTenantAndKB(t *testing.T) {
	_, err := ResolveScope(context.Background(), "kb-a")
	require.True(t, errors.Is(err, ErrNoLearningScope))
	_, err = ResolveScope(learningWebContext(1, "alice"), "")
	require.True(t, errors.Is(err, ErrInvalidKnowledgeBase))
}

func TestDisplayedReferenceExposureMapsOneToManyAndPreservesMastery(t *testing.T) {
	svc, repo, db := newExposureServiceTest(t)
	ownerCtx := learningWebContext(9, "alice")
	profile, err := svc.SetTracking(ownerCtx, "kb-shared", true)
	require.NoError(t, err)

	// Pre-existing verified state proves exposure aggregation cannot overwrite
	// quiz-derived fields or status.
	require.NoError(t, db.Create(&types.UserConceptState{
		ID: "state-stable-a", TenantID: 9, SubjectID: "web_user:alice", KnowledgeBaseID: "kb-shared",
		ConceptKey: "stable-a", VerifiedMastery: 0.75, MasteryConfidence: 0.8,
		QuizAttemptCount: 3, Status: types.LearningProfileStatusVerifiedStrong,
	}).Error)
	pageID := "page-a"
	require.NoError(t, repo.CreateConceptIdentity(ownerCtx, &types.LearningConceptIdentity{
		ConceptKey: "stable-a", TenantID: 9, KnowledgeBaseID: "kb-shared", CurrentWikiPageID: &pageID,
		Slug: "concept/a", Title: "A", LearningEligible: true,
	}))

	// The request can originate in another tenant; shared-KB evidence is stored
	// under the KB owner's tenant so the KBAccessRead effective context can read it.
	requestCtx := learningWebContext(1, "alice")
	message := displayedMessage("message-1", profile.EnabledAt.Add(time.Second), "chunk-single", "chunk-multi")
	require.NoError(t, svc.RecordDisplayedReferences(requestCtx, message))

	states, err := svc.ListConceptStates(ownerCtx, "kb-shared")
	require.NoError(t, err)
	require.Len(t, states, 2)
	byConcept := make(map[string]*types.UserConceptState, len(states))
	for _, state := range states {
		byConcept[state.ConceptKey] = state
	}
	stateA := byConcept["stable-a"]
	require.NotNil(t, stateA)
	require.Equal(t, 1, stateA.ExposureCount, "two chunks in one assistant message count as one exposure")
	require.InDelta(t, 1.5, stateA.ExposureWeight, 1e-9)
	require.Equal(t, 0.75, stateA.VerifiedMastery)
	require.Equal(t, 0.8, stateA.MasteryConfidence)
	require.Equal(t, 3, stateA.QuizAttemptCount)
	require.Equal(t, types.LearningProfileStatusVerifiedStrong, stateA.Status)

	var evidence []types.LearningEvidence
	require.NoError(t, db.Order("chunk_id ASC, concept_key ASC").Find(&evidence).Error)
	require.Len(t, evidence, 3)
	confidences := map[string][]float64{}
	for _, item := range evidence {
		confidences[*item.ChunkID] = append(confidences[*item.ChunkID], item.Confidence)
		require.Equal(t, types.LearningEvidenceDisplayedReference, item.EventType)
		require.Equal(t, "message-1", *item.MessageID)
		require.Equal(t, uint64(9), item.TenantID)
	}
	require.Equal(t, []float64{1}, confidences["chunk-single"])
	require.ElementsMatch(t, []float64{0.5, 0.5}, confidences["chunk-multi"])
}

func TestDisplayedReferenceExposureReplayIsIdempotent(t *testing.T) {
	svc, _, db := newExposureServiceTest(t)
	ctx := learningWebContext(9, "alice")
	profile, err := svc.SetTracking(ctx, "kb-shared", true)
	require.NoError(t, err)
	message := displayedMessage("message-replay", profile.EnabledAt.Add(time.Second), "chunk-multi")
	require.NoError(t, svc.RecordDisplayedReferences(ctx, message))
	require.NoError(t, svc.RecordDisplayedReferences(ctx, message))

	states, err := svc.ListConceptStates(ctx, "kb-shared")
	require.NoError(t, err)
	require.Len(t, states, 2)
	for _, state := range states {
		require.Equal(t, 1, state.ExposureCount)
		require.InDelta(t, 0.5, state.ExposureWeight, 1e-9)
	}
	var count int64
	require.NoError(t, db.Model(&types.LearningEvidence{}).Count(&count).Error)
	require.EqualValues(t, 2, count)
}

func TestDisplayedReferenceExposureHonorsTrackingDeletionClearAndUserIsolation(t *testing.T) {
	svc, _, db := newExposureServiceTest(t)
	aliceCtx := learningWebContext(9, "alice")
	bobCtx := learningWebContext(9, "bob")
	profile, err := svc.SetTracking(aliceCtx, "kb-shared", true)
	require.NoError(t, err)

	// A missing/deleted chunk is omitted even if a stale reference reaches the
	// completion hook.
	require.NoError(t, svc.RecordDisplayedReferences(aliceCtx,
		displayedMessage("missing", profile.EnabledAt.Add(time.Second), "chunk-missing")))
	var count int64
	require.NoError(t, db.Model(&types.LearningEvidence{}).Count(&count).Error)
	require.Zero(t, count)

	// Bob has not enabled tracking, so the same completion creates no state for
	// him and cannot see Alice's future state.
	require.NoError(t, svc.RecordDisplayedReferences(bobCtx,
		displayedMessage("bob-disabled", profile.EnabledAt.Add(time.Second), "chunk-single")))
	bobStates, err := svc.ListConceptStates(bobCtx, "kb-shared")
	require.NoError(t, err)
	require.Empty(t, bobStates)

	cleared, err := svc.Clear(aliceCtx, "kb-shared")
	require.NoError(t, err)
	require.NoError(t, svc.RecordDisplayedReferences(aliceCtx,
		displayedMessage("late", *cleared.ClearedAt, "chunk-single")))
	aliceStates, err := svc.ListConceptStates(aliceCtx, "kb-shared")
	require.NoError(t, err)
	require.Empty(t, aliceStates)
	require.NoError(t, db.Model(&types.LearningEvidence{}).Count(&count).Error)
	require.Zero(t, count)
}
