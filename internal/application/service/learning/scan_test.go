package learning

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

func addScanConcept(t *testing.T, fixture *quizFixture, key, pageID, title string, degree int) {
	t.Helper()
	page := &types.WikiPage{ID: pageID, TenantID: 7, KnowledgeBaseID: "kb-a", Slug: "concept/" + key,
		Title: title, Summary: fixture.page.Summary, PageType: types.WikiPageTypeConcept,
		Status: types.WikiPageStatusPublished, ChunkRefs: fixture.page.ChunkRefs,
		InLinks: make(types.StringArray, degree)}
	fixture.service.wikiRepo.(*quizWikiRepository).pages[pageID] = page
	require.NoError(t, fixture.repo.CreateConceptIdentity(context.Background(), &types.LearningConceptIdentity{
		ConceptKey: key, TenantID: 7, KnowledgeBaseID: "kb-a", CurrentWikiPageID: ptr(pageID),
		Slug: page.Slug, Title: title, LearningEligible: true, Status: types.LearningConceptIdentityActive,
	}))
	items := make([]types.QuizItem, 0, 4)
	for i := 0; i < 4; i++ {
		items = append(items, types.QuizItem{ID: fmt.Sprintf("%s-item-%d", key, i), TenantID: 7,
			KnowledgeBaseID: "kb-a", ConceptKey: key, Question: fmt.Sprintf("%s question %d", title, i),
			QuestionHash: fmt.Sprintf("%s-question-%d", key, i), Options: types.StringArray{"a", "b", "c", "d"},
			CorrectOption: 0, Explanation: "grounded", SourceHash: hashQuizSource([]*types.Chunk{
				fixture.chunks.chunks["chunk-a"], fixture.chunks.chunks["chunk-b"],
			}), IsActive: true})
	}
	require.NoError(t, fixture.db.Create(&items).Error)
}

func newScanFixture(t *testing.T) *quizFixture {
	t.Helper()
	fixture := newQuizFixture(t, validQuizJSON("unused", 4))
	addScanConcept(t, fixture, "concept-b", "page-b", "B", 2)
	addScanConcept(t, fixture, "concept-c", "page-c", "C", 1)
	items := make([]types.QuizItem, 0, 4)
	for i := 0; i < 4; i++ {
		items = append(items, types.QuizItem{ID: fmt.Sprintf("rag-item-%d", i), TenantID: 7,
			KnowledgeBaseID: "kb-a", ConceptKey: "concept-rag", Question: fmt.Sprintf("RAG question %d", i),
			QuestionHash: fmt.Sprintf("rag-question-%d", i), Options: types.StringArray{"a", "b", "c", "d"},
			CorrectOption: 0, Explanation: "grounded", SourceHash: hashQuizSource([]*types.Chunk{
				fixture.chunks.chunks["chunk-a"], fixture.chunks.chunks["chunk-b"],
			}), IsActive: true})
	}
	require.NoError(t, fixture.db.Create(&items).Error)
	_, err := fixture.service.SetTracking(learningWebContext(7, "alice"), "kb-a", true)
	require.NoError(t, err)
	return fixture
}

func TestScanCreatesThreeConceptsWithTwoDistinctItemsAndResumes(t *testing.T) {
	fixture := newScanFixture(t)
	ctx := learningWebContext(7, "alice")
	scan, err := fixture.service.StartOrResumeScan(ctx, "kb-a")
	require.NoError(t, err)
	require.Len(t, scan.Items, 6)
	perConcept := map[string]int{}
	seen := map[string]bool{}
	for _, item := range scan.Items {
		perConcept[item.ConceptKey]++
		require.False(t, seen[item.ID])
		seen[item.ID] = true
	}
	require.Len(t, perConcept, 3)
	for _, count := range perConcept {
		require.Equal(t, 2, count)
	}
	resumed, err := fixture.service.StartOrResumeScan(ctx, "kb-a")
	require.NoError(t, err)
	require.Equal(t, scan.ID, resumed.ID)
}

func TestScanPrefersSeenConceptsAndKeepsCallersIsolated(t *testing.T) {
	fixture := newScanFixture(t)
	ctx := learningWebContext(7, "alice")
	scope := interfaces.LearningScope{TenantID: 7, SubjectID: "web_user:alice", KnowledgeBaseID: "kb-a"}
	require.NoError(t, fixture.db.Create(&types.UserConceptState{ID: "state-b", TenantID: 7, SubjectID: scope.SubjectID,
		KnowledgeBaseID: "kb-a", ConceptKey: "concept-b", ExposureWeight: 3, Status: types.LearningProfileStatusExposed}).Error)
	scan, err := fixture.service.StartOrResumeScan(ctx, "kb-a")
	require.NoError(t, err)
	require.Equal(t, "concept-b", scan.Items[0].ConceptKey)
	bobCtx := learningWebContext(7, "bob")
	_, err = fixture.service.GetActiveScan(bobCtx, "kb-a")
	require.NoError(t, err)
	bobScan, err := fixture.service.StartOrResumeScan(bobCtx, "kb-a")
	require.ErrorIs(t, err, ErrLearningTrackingDisabled)
	require.Nil(t, bobScan)
}

func TestScanAttemptReplayDoesNotAdvanceProgressTwice(t *testing.T) {
	fixture := newScanFixture(t)
	ctx := learningWebContext(7, "alice")
	scan, err := fixture.service.StartOrResumeScan(ctx, "kb-a")
	require.NoError(t, err)
	item := scan.Items[0]
	_, err = fixture.service.SubmitQuizAttempt(ctx, "kb-a", item.ID, types.QuizAttemptRequest{
		ScanID: scan.ID, SelectedOption: 0, IdempotencyKey: "scan-replay",
	})
	require.NoError(t, err)
	_, err = fixture.service.SubmitQuizAttempt(ctx, "kb-a", item.ID, types.QuizAttemptRequest{
		ScanID: scan.ID, SelectedOption: 0, IdempotencyKey: "scan-replay",
	})
	require.NoError(t, err)
	stored, err := fixture.repo.GetScan(ctx, interfaces.LearningScope{TenantID: 7, SubjectID: "web_user:alice", KnowledgeBaseID: "kb-a"}, scan.ID)
	require.NoError(t, err)
	require.Equal(t, 1, stored.CurrentIndex)
	_, err = fixture.service.SubmitQuizAttempt(ctx, "kb-a", item.ID, types.QuizAttemptRequest{
		ScanID: scan.ID, SelectedOption: 0, IdempotencyKey: "different-key",
	})
	require.ErrorIs(t, err, ErrInvalidQuizAttempt)
}

func TestScanBackfillsPartialAndNewPublishedConcepts(t *testing.T) {
	f := newScanFixture(t)
	ctx := learningWebContext(7, "alice")
	require.NoError(t, f.db.Where("concept_key IN ?", []string{"concept-b", "concept-c"}).Delete(&types.LearningConceptIdentity{}).Error)
	scope := interfaces.LearningScope{TenantID: 7, SubjectID: "web_user:alice", KnowledgeBaseID: "kb-a"}
	kb := &types.KnowledgeBase{ID: "kb-a", TenantID: 7, SummaryModelID: "model-1"}
	candidates, err := f.service.scanCandidates(ctx, scope, kb)
	require.NoError(t, err)
	require.Len(t, candidates, 3)
	identities, err := f.repo.ListConceptIdentities(ctx, 7, "kb-a")
	require.NoError(t, err)
	require.Len(t, identities, 3)
	original, err := f.repo.GetConceptIdentity(ctx, 7, "kb-a", "concept-rag")
	require.NoError(t, err)
	require.Equal(t, "page-rag", *original.CurrentWikiPageID)
	page := *f.page
	page.ID, page.Slug, page.Title = "page-new", "concept/new", "New concept"
	f.service.wikiRepo.(*quizWikiRepository).pages[page.ID] = &page
	candidates, err = f.service.scanCandidates(ctx, scope, kb)
	require.NoError(t, err)
	require.Len(t, candidates, 4)
	_, err = f.service.scanCandidates(ctx, scope, kb)
	require.NoError(t, err)
	identities, err = f.repo.ListConceptIdentities(ctx, 7, "kb-a")
	require.NoError(t, err)
	require.Len(t, identities, 4, "repeated reconciliation must not duplicate identities")
}

func TestScanBackfillsEmptyIdentitiesAndIgnoresUnpublishedPages(t *testing.T) {
	f := newScanFixture(t)
	ctx := learningWebContext(7, "alice")
	require.NoError(t, f.db.Where("tenant_id = ?", 7).Delete(&types.LearningConceptIdentity{}).Error)
	f.service.wikiRepo.(*quizWikiRepository).pages["page-b"].Status = "draft"
	candidates, err := f.service.scanCandidates(ctx, interfaces.LearningScope{TenantID: 7, SubjectID: "web_user:alice", KnowledgeBaseID: "kb-a"}, &types.KnowledgeBase{ID: "kb-a", TenantID: 7, SummaryModelID: "model-1"})
	require.NoError(t, err)
	require.Len(t, candidates, 2)
}

func TestConceptRetestResumesIndependentlyAndUpdatesOnlyTarget(t *testing.T) {
	f := newScanFixture(t)
	ctx := learningWebContext(7, "alice")
	full, err := f.service.StartOrResumeScan(ctx, "kb-a")
	require.NoError(t, err)
	retest, err := f.service.StartOrResumeConceptScan(ctx, "kb-a", "concept-b")
	require.NoError(t, err)
	require.Len(t, retest.Items, 2)
	require.NotEqual(t, full.ID, retest.ID)
	for _, item := range retest.Items {
		require.Equal(t, "concept-b", item.ConceptKey)
	}
	other, err := f.service.StartOrResumeConceptScan(ctx, "kb-a", "concept-c")
	require.NoError(t, err)
	require.NotEqual(t, retest.ID, other.ID)
	resumed, err := f.service.StartOrResumeConceptScan(ctx, "kb-a", "concept-b")
	require.NoError(t, err)
	require.Equal(t, retest.ID, resumed.ID)
	active, err := f.service.GetActiveScan(ctx, "kb-a")
	require.NoError(t, err)
	require.Equal(t, full.ID, active.ID)
	for i, item := range retest.Items {
		_, err = f.service.SubmitQuizAttempt(ctx, "kb-a", item.ID, types.QuizAttemptRequest{ScanID: retest.ID, SelectedOption: 0, IdempotencyKey: fmt.Sprintf("retest-%d", i)})
		require.NoError(t, err)
	}
	completed, err := f.service.CompleteScan(ctx, "kb-a", retest.ID)
	require.NoError(t, err)
	require.Equal(t, types.LearningScanStatusCompleted, completed.Status)
	require.Equal(t, 1, completed.Summary.VerifiedStrong)
	states, err := f.repo.ListConceptStates(ctx, interfaces.LearningScope{TenantID: 7, SubjectID: "web_user:alice", KnowledgeBaseID: "kb-a"})
	require.NoError(t, err)
	require.Len(t, states, 1)
	require.Equal(t, "concept-b", states[0].ConceptKey)
	next, err := f.service.StartOrResumeConceptScan(ctx, "kb-a", "concept-b")
	require.NoError(t, err)
	require.NotEqual(t, retest.ID, next.ID)
	for _, item := range next.Items {
		for _, old := range retest.Items {
			require.NotEqual(t, old.ID, item.ID, "prefer unseen bank items")
		}
	}
	active, err = f.service.GetActiveScan(ctx, "kb-a")
	require.NoError(t, err)
	require.Equal(t, full.ID, active.ID)
	require.Zero(t, active.CurrentIndex)
}

func TestConceptRetestRequiresValidScopedEligibleConcept(t *testing.T) {
	f := newScanFixture(t)
	ctx := learningWebContext(7, "alice")
	for _, key := range []string{"", "missing"} {
		scan, err := f.service.StartOrResumeConceptScan(ctx, "kb-a", key)
		require.Error(t, err)
		require.Nil(t, scan)
	}
	_, err := f.service.StartOrResumeConceptScan(learningWebContext(7, "bob"), "kb-a", "concept-b")
	require.ErrorIs(t, err, ErrLearningTrackingDisabled)
	_, err = f.service.StartOrResumeConceptScan(ctx, "kb-other", "concept-b")
	require.Error(t, err)
	require.NoError(t, f.db.Model(&types.LearningConceptIdentity{}).Where("concept_key = ?", "concept-b").Update("is_learning_eligible", false).Error)
	_, err = f.service.StartOrResumeConceptScan(ctx, "kb-a", "concept-b")
	require.Error(t, err)
}

func TestConcurrentConceptRetestsResumeOneScan(t *testing.T) {
	f := newScanFixture(t)
	ctx := learningWebContext(7, "alice")
	var wg sync.WaitGroup
	scans := make([]*types.LearningScanView, 8)
	errs := make([]error, 8)
	for i := range scans {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			scans[i], errs[i] = f.service.StartOrResumeConceptScan(ctx, "kb-a", "concept-b")
		}(i)
	}
	wg.Wait()
	for i := range scans {
		require.NoError(t, errs[i])
		require.Equal(t, scans[0].ID, scans[i].ID)
	}
	var count int64
	require.NoError(t, f.db.Model(&types.LearningScan{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
}

func TestPersonalMapBackfillsPartialIdentitiesBeforeAnyScan(t *testing.T) {
	f := newScanFixture(t)
	require.NoError(t, f.db.Where("concept_key IN ?", []string{"concept-b", "concept-c"}).Delete(&types.LearningConceptIdentity{}).Error)
	overlay, err := f.service.GetLearningOverlay(learningWebContext(7, "alice"), "kb-a")
	require.NoError(t, err)
	require.Len(t, overlay.Items, 3)
	var count int64
	require.NoError(t, f.db.Model(&types.LearningScan{}).Count(&count).Error)
	require.Zero(t, count, "opening a map must not create a scan")
	for _, item := range overlay.Items {
		require.NotEmpty(t, item.ConceptKey)
	}
}

func TestBackfillDoesNotStealIdentityFromLivePageWithSameName(t *testing.T) {
	f := newScanFixture(t)
	page := *f.page
	page.ID, page.Slug = "page-same-name", "concept/rag-other"
	f.service.wikiRepo.(*quizWikiRepository).pages[page.ID] = &page
	ctx := learningWebContext(7, "alice")
	overlay, err := f.service.GetLearningOverlay(ctx, "kb-a")
	require.NoError(t, err)
	require.Len(t, overlay.Items, 4)
	original, err := f.repo.GetConceptIdentity(ctx, 7, "kb-a", "concept-rag")
	require.NoError(t, err)
	require.Equal(t, f.page.ID, *original.CurrentWikiPageID)
	overlay, err = f.service.GetLearningOverlay(ctx, "kb-a")
	require.NoError(t, err)
	require.Len(t, overlay.Items, 4)
}
