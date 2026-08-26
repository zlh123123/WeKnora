package learning

import (
	"context"
	"fmt"
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
