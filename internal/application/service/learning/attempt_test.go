package learning

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type attemptFixture struct {
	*quizFixture
	ctx     context.Context
	items   []types.QuizItem
	scan    types.LearningScan
	subject string
}

func newAttemptFixture(t *testing.T, tracking bool) *attemptFixture {
	t.Helper()
	fixture := newQuizFixture(t, validQuizJSON("unused", 4))
	ctx := learningWebContext(7, "alice")
	if tracking {
		_, err := fixture.service.SetTracking(ctx, "kb-a", true)
		require.NoError(t, err)
	}
	items := []types.QuizItem{
		{ID: "item-1", TenantID: 7, KnowledgeBaseID: "kb-a", ConceptKey: "concept-rag", Question: "q1", QuestionHash: "question-1", Options: types.StringArray{"a", "b", "c", "d"}, CorrectOption: 1, Explanation: "because b", SourceHash: "hash-current", IsActive: true},
		{ID: "item-2", TenantID: 7, KnowledgeBaseID: "kb-a", ConceptKey: "concept-rag", Question: "q2", QuestionHash: "question-2", Options: types.StringArray{"a", "b", "c", "d"}, CorrectOption: 2, Explanation: "because c", SourceHash: "hash-current", IsActive: true},
	}
	require.NoError(t, fixture.db.Create(&items).Error)
	scan := types.LearningScan{
		ID: "scan-a", TenantID: 7, SubjectID: "web_user:alice", KnowledgeBaseID: "kb-a",
		Status: types.LearningScanStatusActive, QuizItemIDs: types.StringArray{"item-1", "item-2"},
	}
	require.NoError(t, fixture.db.Create(&scan).Error)
	return &attemptFixture{quizFixture: fixture, ctx: ctx, items: items, scan: scan, subject: "web_user:alice"}
}

func submitAnswer(t *testing.T, fixture *attemptFixture, item int, selected int, key string) *types.QuizAttemptResult {
	t.Helper()
	result, err := fixture.service.SubmitQuizAttempt(
		fixture.ctx, "kb-a", fixture.items[item].ID,
		types.QuizAttemptRequest{ScanID: fixture.scan.ID, SelectedOption: selected, IdempotencyKey: key},
	)
	require.NoError(t, err)
	return result
}

func TestQuizAttemptDeterministicGradingAndMasteryThresholds(t *testing.T) {
	t.Run("correct then strong", func(t *testing.T) {
		fixture := newAttemptFixture(t, true)
		first := submitAnswer(t, fixture, 0, 1, "correct-1")
		require.True(t, first.IsCorrect)
		require.Equal(t, "because b", first.Explanation)
		require.Equal(t, types.LearningProfileStatusUnseen, first.PreviousState.Status)
		require.Equal(t, types.LearningProfileStatusUncertain, first.NewState.Status)
		require.Equal(t, 1.0, first.VerifiedMastery)
		require.Equal(t, 0.25, first.MasteryConfidence)

		second := submitAnswer(t, fixture, 1, 2, "correct-2")
		require.True(t, second.IsCorrect)
		require.Equal(t, types.LearningProfileStatusVerifiedStrong, second.NewState.Status)
		require.Equal(t, 1.0, second.VerifiedMastery)
		require.Equal(t, 0.5, second.MasteryConfidence)
	})

	t.Run("wrong then weak", func(t *testing.T) {
		fixture := newAttemptFixture(t, true)
		first := submitAnswer(t, fixture, 0, 0, "wrong-1")
		require.False(t, first.IsCorrect)
		require.Equal(t, types.LearningProfileStatusUncertain, first.NewState.Status)
		second := submitAnswer(t, fixture, 1, 0, "wrong-2")
		require.False(t, second.IsCorrect)
		require.Equal(t, types.LearningProfileStatusVerifiedWeak, second.NewState.Status)
		require.Zero(t, second.VerifiedMastery)
	})

	t.Run("one of two remains uncertain", func(t *testing.T) {
		fixture := newAttemptFixture(t, true)
		submitAnswer(t, fixture, 0, 1, "mixed-1")
		second := submitAnswer(t, fixture, 1, 0, "mixed-2")
		require.Equal(t, types.LearningProfileStatusUncertain, second.NewState.Status)
		require.Equal(t, 0.5, second.VerifiedMastery)
	})
}

func TestQuizAttemptIsIdempotentAndEvidenceIsOneToOne(t *testing.T) {
	fixture := newAttemptFixture(t, true)
	first := submitAnswer(t, fixture, 0, 1, "same-request")
	retry := submitAnswer(t, fixture, 0, 0, "same-request")
	require.Equal(t, first.AttemptID, retry.AttemptID)
	require.True(t, retry.Idempotent)
	require.Equal(t, first.IsCorrect, retry.IsCorrect)
	require.Equal(t, first.PreviousState, retry.PreviousState)
	require.Equal(t, first.NewState, retry.NewState)

	var attempts, evidence int64
	require.NoError(t, fixture.db.Model(&types.QuizAttempt{}).Count(&attempts).Error)
	require.NoError(t, fixture.db.Model(&types.LearningEvidence{}).
		Where("event_type = ?", types.LearningEvidenceQuizAttempt).Count(&evidence).Error)
	require.EqualValues(t, 1, attempts)
	require.EqualValues(t, attempts, evidence)
}

func TestConcurrentQuizAttemptsDoNotLoseStateUpdates(t *testing.T) {
	fixture := newAttemptFixture(t, true)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := fixture.service.SubmitQuizAttempt(
				fixture.ctx, "kb-a", fixture.items[index].ID,
				types.QuizAttemptRequest{ScanID: fixture.scan.ID, SelectedOption: fixture.items[index].CorrectOption, IdempotencyKey: fmt.Sprintf("parallel-%d", index)},
			)
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	state, err := fixture.repo.GetConceptState(fixture.ctx, interfaceScope(7, fixture.subject, "kb-a"), "concept-rag")
	require.NoError(t, err)
	require.Equal(t, 2, state.QuizAttemptCount)
	require.Equal(t, types.LearningProfileStatusVerifiedStrong, state.Status)
}

func interfaceScope(tenant uint64, subject, kb string) interfaces.LearningScope {
	return interfaces.LearningScope{TenantID: tenant, SubjectID: subject, KnowledgeBaseID: kb}
}

func TestQuizAttemptRejectsWrongSubjectAndUnavailableItems(t *testing.T) {
	fixture := newAttemptFixture(t, true)

	bobScan := types.LearningScan{
		ID: "scan-bob", TenantID: 7, SubjectID: "web_user:bob", KnowledgeBaseID: "kb-a",
		Status: types.LearningScanStatusActive, QuizItemIDs: types.StringArray{"item-1"},
	}
	require.NoError(t, fixture.db.Create(&bobScan).Error)
	_, err := fixture.service.SubmitQuizAttempt(fixture.ctx, "kb-a", "item-1", types.QuizAttemptRequest{
		ScanID: bobScan.ID, SelectedOption: 1, IdempotencyKey: "bob-scan",
	})
	require.ErrorIs(t, err, ErrLearningScanUnavailable)

	require.NoError(t, fixture.db.Model(&types.QuizItem{}).Where("id = ?", "item-1").Update("is_active", false).Error)
	_, err = fixture.service.SubmitQuizAttempt(fixture.ctx, "kb-a", "item-1", types.QuizAttemptRequest{
		ScanID: fixture.scan.ID, SelectedOption: 1, IdempotencyKey: "inactive",
	})
	require.ErrorIs(t, err, ErrQuizItemUnavailable)

	require.NoError(t, fixture.db.Model(&types.QuizItem{}).Where("id = ?", "item-2").Update("is_stale", true).Error)
	_, err = fixture.service.SubmitQuizAttempt(fixture.ctx, "kb-a", "item-2", types.QuizAttemptRequest{
		ScanID: fixture.scan.ID, SelectedOption: 2, IdempotencyKey: "stale",
	})
	require.ErrorIs(t, err, ErrQuizItemUnavailable)
}

func TestQuizAttemptPreservesExposureProjection(t *testing.T) {
	fixture := newAttemptFixture(t, true)
	exposedAt := time.Now().Add(-time.Hour).Truncate(time.Millisecond)
	require.NoError(t, fixture.db.Create(&types.UserConceptState{
		ID: "state-exposed", TenantID: 7, SubjectID: fixture.subject, KnowledgeBaseID: "kb-a",
		ConceptKey: "concept-rag", ExposureCount: 3, ExposureWeight: 1.75,
		LastExposedAt: &exposedAt, Status: types.LearningProfileStatusExposed,
	}).Error)
	submitAnswer(t, fixture, 0, 1, "preserve-exposure")
	state, err := fixture.repo.GetConceptState(fixture.ctx, interfaceScope(7, fixture.subject, "kb-a"), "concept-rag")
	require.NoError(t, err)
	require.Equal(t, 3, state.ExposureCount)
	require.Equal(t, 1.75, state.ExposureWeight)
	require.WithinDuration(t, exposedAt, *state.LastExposedAt, time.Millisecond)
}

func TestQuizAttemptUsesOnlyCurrentNonStaleSourceHash(t *testing.T) {
	fixture := newAttemptFixture(t, true)
	old := types.QuizItem{ID: "old-item", TenantID: 7, KnowledgeBaseID: "kb-a", ConceptKey: "concept-rag", Question: "old", QuestionHash: "question-old", Options: types.StringArray{"a", "b", "c", "d"}, CorrectOption: 0, SourceHash: "hash-old", IsActive: true}
	require.NoError(t, fixture.db.Create(&old).Error)
	fixture.scan.QuizItemIDs = append(fixture.scan.QuizItemIDs, old.ID)
	require.NoError(t, fixture.db.Model(&types.LearningScan{}).Where("id = ?", fixture.scan.ID).Update("quiz_item_ids", fixture.scan.QuizItemIDs).Error)
	oldResult, err := fixture.service.SubmitQuizAttempt(fixture.ctx, "kb-a", old.ID, types.QuizAttemptRequest{ScanID: fixture.scan.ID, SelectedOption: 1, IdempotencyKey: "old-wrong"})
	require.NoError(t, err)
	require.False(t, oldResult.IsCorrect)
	current := submitAnswer(t, fixture, 0, 1, "new-correct")
	require.Equal(t, 1.0, current.VerifiedMastery)
	require.Equal(t, types.LearningProfileStatusUncertain, current.NewState.Status)
	require.Equal(t, 2, current.NewState.QuizAttemptCount)
}

func TestQuizAttemptRequiresTracking(t *testing.T) {
	fixture := newAttemptFixture(t, false)
	_, err := fixture.service.SubmitQuizAttempt(fixture.ctx, "kb-a", "item-1", types.QuizAttemptRequest{
		ScanID: fixture.scan.ID, SelectedOption: 1, IdempotencyKey: "tracking-off",
	})
	require.ErrorIs(t, err, ErrLearningTrackingDisabled)
}
