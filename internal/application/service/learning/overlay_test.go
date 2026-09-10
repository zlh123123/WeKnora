package learning

import (
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
)

func TestLearningOverlayIsCallerScopedAndDefaultsMissingStateToUnseen(t *testing.T) {
	svc, repo, db := newExposureServiceTest(t)
	pageA, pageB, orphanPage := "page-a", "page-b", "page-orphan"
	for _, identity := range []*types.LearningConceptIdentity{
		{ConceptKey: "concept-a", TenantID: 9, KnowledgeBaseID: "kb-shared", CurrentWikiPageID: &pageA, Slug: "concept/a", Title: "A", LearningEligible: true, Status: types.LearningConceptIdentityActive},
		{ConceptKey: "concept-b", TenantID: 9, KnowledgeBaseID: "kb-shared", CurrentWikiPageID: &pageB, Slug: "concept/b", Title: "B", LearningEligible: true, Status: types.LearningConceptIdentityActive},
		{ConceptKey: "concept-orphan", TenantID: 9, KnowledgeBaseID: "kb-shared", CurrentWikiPageID: &orphanPage, Slug: "concept/orphan", Title: "Orphan", LearningEligible: true, Status: types.LearningConceptIdentityOrphaned},
	} {
		require.NoError(t, repo.CreateConceptIdentity(learningWebContext(9, "alice"), identity))
	}
	assessedAt := time.Now().Truncate(time.Millisecond)
	require.NoError(t, db.Create(&types.UserConceptState{
		ID: "state-alice-a", TenantID: 9, SubjectID: "web_user:alice", KnowledgeBaseID: "kb-shared",
		ConceptKey: "concept-a", ExposureCount: 4, ExposureWeight: 2.5, VerifiedMastery: 1,
		MasteryConfidence: .5, QuizAttemptCount: 2, LastAssessedAt: &assessedAt,
		Status: types.LearningProfileStatusVerifiedStrong,
	}).Error)
	require.NoError(t, db.Create(&types.UserConceptState{
		ID: "state-bob-a", TenantID: 9, SubjectID: "web_user:bob", KnowledgeBaseID: "kb-shared",
		ConceptKey: "concept-a", VerifiedMastery: 0, MasteryConfidence: .5,
		QuizAttemptCount: 2, Status: types.LearningProfileStatusVerifiedWeak,
	}).Error)
	_, err := svc.SetTracking(learningWebContext(9, "alice"), "kb-shared", true)
	require.NoError(t, err)

	overlay, err := svc.GetLearningOverlay(learningWebContext(1, "alice"), "kb-shared")
	require.NoError(t, err)
	require.True(t, overlay.TrackingEnabled)
	require.Len(t, overlay.Items, 2, "orphaned identity must be skipped without failing the batch")
	require.Equal(t, "concept-a", overlay.Items[0].ConceptKey)
	require.Equal(t, "page-a", overlay.Items[0].WikiPageID)
	require.Equal(t, types.LearningProfileStatusVerifiedStrong, overlay.Items[0].Status)
	require.Equal(t, 4, overlay.Items[0].ExposureCount)
	require.Equal(t, 1.0, overlay.Items[0].VerifiedMastery)
	require.Equal(t, 2, overlay.Items[0].QuizAttemptCount)
	require.Equal(t, types.LearningProfileStatusUnseen, overlay.Items[1].Status)
	require.Zero(t, overlay.Items[1].ExposureCount)

	bobOverlay, err := svc.GetLearningOverlay(learningWebContext(9, "bob"), "kb-shared")
	require.NoError(t, err)
	require.False(t, bobOverlay.TrackingEnabled)
	require.Equal(t, types.LearningProfileStatusVerifiedWeak, bobOverlay.Items[0].Status)
}

func TestLearningOverlayReturnsOnlyRequestedKnowledgeBase(t *testing.T) {
	svc, repo, _ := newExposureServiceTest(t)
	page := "foreign-page"
	require.NoError(t, repo.CreateConceptIdentity(learningWebContext(9, "alice"), &types.LearningConceptIdentity{
		ConceptKey: "foreign", TenantID: 9, KnowledgeBaseID: "kb-other", CurrentWikiPageID: &page,
		Slug: "concept/foreign", Title: "Foreign", LearningEligible: true, Status: types.LearningConceptIdentityActive,
	}))
	overlay, err := svc.GetLearningOverlay(learningWebContext(9, "alice"), "kb-shared")
	require.NoError(t, err)
	require.Len(t, overlay.Items, 2)
	for _, item := range overlay.Items {
		require.NotEqual(t, "foreign", item.ConceptKey)
		require.Contains(t, []string{"page-a", "page-b"}, item.WikiPageID)
	}
}

func TestConceptInsightsClassifiesGapAndRanksRecommendationDeterministically(t *testing.T) {
	svc, repo, db := newExposureServiceTest(t)
	pageA, pageB := "page-a", "page-b"
	for _, identity := range []*types.LearningConceptIdentity{
		{ConceptKey: "concept-a", TenantID: 9, KnowledgeBaseID: "kb-shared", CurrentWikiPageID: &pageA, Slug: "concept/a", Title: "A", LearningEligible: true, Status: types.LearningConceptIdentityActive},
		{ConceptKey: "concept-b", TenantID: 9, KnowledgeBaseID: "kb-shared", CurrentWikiPageID: &pageB, Slug: "concept/b", Title: "B", LearningEligible: true, Status: types.LearningConceptIdentityActive},
	} {
		require.NoError(t, repo.CreateConceptIdentity(learningWebContext(9, "alice"), identity))
	}
	require.NoError(t, db.Create(&types.UserConceptState{ID: "gap", TenantID: 9, SubjectID: "web_user:alice", KnowledgeBaseID: "kb-shared", ConceptKey: "concept-a", ExposureCount: 4, ExposureWeight: 2, QuizAttemptCount: 2, Status: types.LearningProfileStatusVerifiedWeak}).Error)
	require.NoError(t, db.Create(&types.UserConceptState{ID: "recommend", TenantID: 9, SubjectID: "web_user:alice", KnowledgeBaseID: "kb-shared", ConceptKey: "concept-b", ExposureCount: 2, ExposureWeight: 1, QuizAttemptCount: 1, Status: types.LearningProfileStatusUncertain}).Error)
	now := time.Now().Add(-time.Minute)
	require.NoError(t, db.Create(&types.LearningEvidence{ID: "evidence-a", TenantID: 9, SubjectID: "web_user:alice", KnowledgeBaseID: "kb-shared", ConceptKey: "concept-a", EventType: types.LearningEvidenceDisplayedReference, Confidence: .5, OccurredAt: now, IdempotencyKey: "evidence-a"}).Error)

	insights, err := svc.GetConceptInsights(learningWebContext(9, "alice"), "kb-shared", "concept-a")
	require.NoError(t, err)
	require.True(t, insights.IsKnowledgeGap)
	require.Equal(t, "concept-a", insights.ConceptKey)
	require.Len(t, insights.Evidence, 1)
	require.NotNil(t, insights.Recommendation)
	require.Equal(t, "concept-b", insights.Recommendation.ConceptKey)

	state, err := svc.GetConceptInsights(learningWebContext(9, "alice"), "kb-shared", "concept-b")
	require.NoError(t, err)
	require.False(t, state.IsKnowledgeGap, "uncertain with only two exposures is not a gap")
}
