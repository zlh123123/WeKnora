package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestLearningExportExcludesOtherScopesAndAnswerKeys(t *testing.T) {
	repo, db := newLearningTestRepository(t)
	ctx := context.Background()
	scopes := []interfaces.LearningScope{
		{TenantID: 1, SubjectID: "web_user:a", KnowledgeBaseID: "kb-a"},
		{TenantID: 1, SubjectID: "web_user:b", KnowledgeBaseID: "kb-a"},
		{TenantID: 2, SubjectID: "web_user:a", KnowledgeBaseID: "kb-a"},
		{TenantID: 1, SubjectID: "web_user:a", KnowledgeBaseID: "kb-b"},
	}
	for i, scope := range scopes {
		id := fmt.Sprintf("export-%d", i)
		require.NoError(t, db.Create(&types.UserConceptState{ID: id, TenantID: scope.TenantID, SubjectID: scope.SubjectID, KnowledgeBaseID: scope.KnowledgeBaseID, ConceptKey: id}).Error)
		require.NoError(t, db.Create(&types.LearningEvidence{ID: id, TenantID: scope.TenantID, SubjectID: scope.SubjectID, KnowledgeBaseID: scope.KnowledgeBaseID, ConceptKey: id, IdempotencyKey: id}).Error)
		require.NoError(t, db.Create(&types.QuizAttempt{ID: id, TenantID: scope.TenantID, SubjectID: scope.SubjectID, KnowledgeBaseID: scope.KnowledgeBaseID, ConceptKey: id, IdempotencyKey: id}).Error)
	}
	for i, scope := range []interfaces.LearningScope{scopes[0], scopes[2], scopes[3]} {
		require.NoError(t, db.Create(&types.QuizItem{ID: fmt.Sprintf("item-%d", i), TenantID: scope.TenantID, KnowledgeBaseID: scope.KnowledgeBaseID, ConceptKey: "concept", Question: "question", Options: types.StringArray{"a", "b", "c", "d"}, CorrectOption: 2, Explanation: "private-answer-explanation"}).Error)
	}
	exported, err := repo.ExportScope(ctx, scopes[0])
	require.NoError(t, err)
	require.Len(t, exported.ConceptStates, 1)
	require.Len(t, exported.Evidence, 1)
	require.Len(t, exported.QuizAttempts, 1)
	require.Len(t, exported.QuizItems, 1)
	require.Equal(t, "export-0", exported.ConceptStates[0].ID)
	require.Equal(t, "export-0", exported.Evidence[0].ID)
	require.Equal(t, "export-0", exported.QuizAttempts[0].ID)
	require.Equal(t, "item-0", exported.QuizItems[0].ID)
	encoded, err := json.Marshal(exported)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "correct_option")
	require.NotContains(t, string(encoded), "private-answer-explanation")
	_, err = repo.ExportScope(ctx, interfaces.LearningScope{})
	require.ErrorIs(t, err, ErrInvalidLearningScope)
}

func newLearningTestRepository(t *testing.T) (interfaces.LearningRepository, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.LearningProfile{},
		&types.LearningConceptIdentity{},
		&types.UserConceptState{},
		&types.QuizItem{},
		&types.LearningScan{},
		&types.QuizAttempt{},
		&types.LearningEvidence{},
	))
	return NewLearningRepository(db), db
}

func TestLearningRepositoryScopesStatesByTenantSubjectAndKB(t *testing.T) {
	repo, db := newLearningTestRepository(t)
	ctx := context.Background()
	scopeA := interfaces.LearningScope{TenantID: 1, SubjectID: "web_user:a", KnowledgeBaseID: "kb-a"}
	require.NoError(t, db.Create(&types.UserConceptState{
		ID: "state-a", TenantID: scopeA.TenantID, SubjectID: scopeA.SubjectID, KnowledgeBaseID: scopeA.KnowledgeBaseID,
		ConceptKey: "concept-1", ExposureCount: 3, Status: types.LearningProfileStatusExposed,
	}).Error)

	tests := []struct {
		name  string
		scope interfaces.LearningScope
	}{
		{name: "other subject", scope: interfaces.LearningScope{TenantID: 1, SubjectID: "web_user:b", KnowledgeBaseID: "kb-a"}},
		{name: "other tenant", scope: interfaces.LearningScope{TenantID: 2, SubjectID: "web_user:a", KnowledgeBaseID: "kb-a"}},
		{name: "other kb", scope: interfaces.LearningScope{TenantID: 1, SubjectID: "web_user:a", KnowledgeBaseID: "kb-b"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			state, err := repo.GetConceptState(ctx, tc.scope, "concept-1")
			require.NoError(t, err)
			require.Nil(t, state)
			states, err := repo.ListConceptStates(ctx, tc.scope)
			require.NoError(t, err)
			require.Empty(t, states)
		})
	}

	state, err := repo.GetConceptState(ctx, scopeA, "concept-1")
	require.NoError(t, err)
	require.NotNil(t, state)
	require.Equal(t, 3, state.ExposureCount)
}

func TestLearningEvidenceIsIdempotentAndHonorsTrackingLifecycle(t *testing.T) {
	repo, _ := newLearningTestRepository(t)
	ctx := context.Background()
	scope := interfaces.LearningScope{TenantID: 1, SubjectID: "web_user:a", KnowledgeBaseID: "kb-a"}

	inserted, err := repo.AppendEvidence(ctx, scope, &types.LearningEvidence{
		ConceptKey: "concept-1", EventType: "test", IdempotencyKey: "disabled-event", OccurredAt: time.Now(),
	})
	require.NoError(t, err)
	require.False(t, inserted, "tracking defaults to disabled")

	profile, err := repo.SetTracking(ctx, scope, true)
	require.NoError(t, err)
	require.True(t, profile.TrackingEnabled)
	require.NotNil(t, profile.EnabledAt)

	eventTime := profile.EnabledAt.Add(time.Second)
	evidence := &types.LearningEvidence{
		ConceptKey: "concept-1", EventType: "test", IdempotencyKey: "same-event", OccurredAt: eventTime,
	}
	inserted, err = repo.AppendEvidence(ctx, scope, evidence)
	require.NoError(t, err)
	require.True(t, inserted)
	inserted, err = repo.AppendEvidence(ctx, scope, &types.LearningEvidence{
		ConceptKey: "concept-1", EventType: "test", IdempotencyKey: "same-event", OccurredAt: eventTime,
	})
	require.NoError(t, err)
	require.False(t, inserted)
	count, err := repo.CountEvidence(ctx, scope)
	require.NoError(t, err)
	require.EqualValues(t, 1, count)
	disabled, err := repo.SetTracking(ctx, scope, false)
	require.NoError(t, err)
	require.False(t, disabled.TrackingEnabled)
	inserted, err = repo.AppendEvidence(ctx, scope, &types.LearningEvidence{
		ConceptKey: "concept-1", EventType: "test", IdempotencyKey: "disabled-after-enable", OccurredAt: time.Now(),
	})
	require.NoError(t, err)
	require.False(t, inserted)
	profile, err = repo.SetTracking(ctx, scope, true)
	require.NoError(t, err)
	require.True(t, profile.TrackingEnabled)

	cleared, err := repo.ClearScope(ctx, scope)
	require.NoError(t, err)
	require.NotNil(t, cleared.ClearedAt)
	require.True(t, cleared.TrackingEnabled, "clear preserves the user's tracking choice")
	count, err = repo.CountEvidence(ctx, scope)
	require.NoError(t, err)
	require.Zero(t, count)

	inserted, err = repo.AppendEvidence(ctx, scope, &types.LearningEvidence{
		ConceptKey: "concept-1", EventType: "test", IdempotencyKey: "late-event", OccurredAt: *cleared.ClearedAt,
	})
	require.NoError(t, err)
	require.False(t, inserted, "an event at or before cleared_at must not resurrect data")
	inserted, err = repo.AppendEvidence(ctx, scope, &types.LearningEvidence{
		ConceptKey: "concept-1", EventType: "test", IdempotencyKey: "future-event", OccurredAt: cleared.ClearedAt.Add(time.Second),
	})
	require.NoError(t, err)
	require.True(t, inserted)
}

func TestLearningClearDeletesOnlySubjectOwnedData(t *testing.T) {
	repo, db := newLearningTestRepository(t)
	ctx := context.Background()
	scopeA := interfaces.LearningScope{TenantID: 1, SubjectID: "web_user:a", KnowledgeBaseID: "kb-a"}
	scopeB := interfaces.LearningScope{TenantID: 1, SubjectID: "web_user:b", KnowledgeBaseID: "kb-a"}
	for _, scope := range []interfaces.LearningScope{scopeA, scopeB} {
		require.NoError(t, db.Create(&types.UserConceptState{
			ID: "state-" + scope.SubjectID, TenantID: scope.TenantID, SubjectID: scope.SubjectID,
			KnowledgeBaseID: scope.KnowledgeBaseID, ConceptKey: "concept-1", Status: types.LearningProfileStatusUnseen,
		}).Error)
		require.NoError(t, db.Create(&types.LearningScan{
			ID: "scan-" + scope.SubjectID, TenantID: scope.TenantID, SubjectID: scope.SubjectID,
			KnowledgeBaseID: scope.KnowledgeBaseID, Status: types.LearningScanStatusPending,
		}).Error)
	}

	_, err := repo.ClearScope(ctx, scopeA)
	require.NoError(t, err)
	statesA, err := repo.ListConceptStates(ctx, scopeA)
	require.NoError(t, err)
	require.Empty(t, statesA)
	statesB, err := repo.ListConceptStates(ctx, scopeB)
	require.NoError(t, err)
	require.Len(t, statesB, 1)
	var scansB int64
	require.NoError(t, db.Model(&types.LearningScan{}).
		Where("tenant_id = ? AND subject_id = ? AND knowledge_base_id = ?", 1, scopeB.SubjectID, "kb-a").
		Count(&scansB).Error)
	require.EqualValues(t, 1, scansB)
}

func TestLearningConceptIdentityResolverOrder(t *testing.T) {
	repo, _ := newLearningTestRepository(t)
	ctx := context.Background()
	pageID := "page-1"
	require.NoError(t, repo.CreateConceptIdentity(ctx, &types.LearningConceptIdentity{
		ConceptKey: "concept-1", TenantID: 1, KnowledgeBaseID: "kb-a", CurrentWikiPageID: &pageID,
		Slug: "Concept/RAG", Title: "Retrieval Augmented Generation",
		Aliases: types.StringArray{"RAG", "检索增强生成"}, LearningEligible: true,
	}))

	tests := []struct {
		name   string
		lookup interfaces.LearningConceptLookup
		method string
	}{
		{name: "page id", lookup: interfaces.LearningConceptLookup{CurrentWikiPageID: "page-1"}, method: interfaces.LearningConceptMatchedPageID},
		{name: "normalized slug", lookup: interfaces.LearningConceptLookup{Slug: "/concept/rag/"}, method: interfaces.LearningConceptMatchedSlug},
		{name: "alias", lookup: interfaces.LearningConceptLookup{Title: " rag "}, method: interfaces.LearningConceptMatchedAlias},
		{name: "title", lookup: interfaces.LearningConceptLookup{Title: "Retrieval   Augmented Generation"}, method: interfaces.LearningConceptMatchedTitle},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resolution, err := repo.ResolveConceptIdentity(ctx, 1, "kb-a", tc.lookup)
			require.NoError(t, err)
			require.NotNil(t, resolution.Identity)
			require.Equal(t, "concept-1", resolution.Identity.ConceptKey)
			require.Equal(t, tc.method, resolution.MatchedBy)
		})
	}

	resolution, err := repo.ResolveConceptIdentity(ctx, 1, "kb-a", interfaces.LearningConceptLookup{Title: "unknown"})
	require.NoError(t, err)
	require.Nil(t, resolution.Identity)
	require.False(t, resolution.Ambiguous)
	require.Equal(t, interfaces.LearningConceptMatchedUnresolved, resolution.MatchedBy)
}

func TestLearningConceptIdentityResolverReportsAmbiguity(t *testing.T) {
	repo, _ := newLearningTestRepository(t)
	ctx := context.Background()
	for _, key := range []string{"concept-1", "concept-2"} {
		require.NoError(t, repo.CreateConceptIdentity(ctx, &types.LearningConceptIdentity{
			ConceptKey: key, TenantID: 1, KnowledgeBaseID: "kb-a", Slug: key,
			Title: key, Aliases: types.StringArray{"shared alias"}, LearningEligible: true,
		}))
	}
	resolution, err := repo.ResolveConceptIdentity(ctx, 1, "kb-a", interfaces.LearningConceptLookup{Title: "shared alias"})
	require.NoError(t, err)
	require.Nil(t, resolution.Identity)
	require.True(t, resolution.Ambiguous)
	require.Equal(t, interfaces.LearningConceptMatchedAmbiguous, resolution.MatchedBy)
}
