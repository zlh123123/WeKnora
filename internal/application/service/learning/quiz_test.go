package learning

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/Tencent/WeKnora/internal/application/repository"
	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type quizWikiRepository struct {
	interfaces.WikiPageRepository
	pages map[string]*types.WikiPage
}

func (r *quizWikiRepository) GetByID(_ context.Context, id string) (*types.WikiPage, error) {
	if page := r.pages[id]; page != nil {
		return page, nil
	}
	return nil, errors.New("page not found")
}

type quizChunkRepository struct {
	interfaces.ChunkRepository
	chunks map[string]*types.Chunk
}

func (r *quizChunkRepository) ListChunksByIDOnly(_ context.Context, ids []string) ([]*types.Chunk, error) {
	result := make([]*types.Chunk, 0, len(ids))
	for _, id := range ids {
		if chunk := r.chunks[id]; chunk != nil {
			result = append(result, chunk)
		}
	}
	return result, nil
}

type quizChat struct {
	responses []string
	calls     int
}

func (m *quizChat) Chat(_ context.Context, _ []chat.Message, _ *chat.ChatOptions) (*types.ChatResponse, error) {
	m.calls++
	index := m.calls - 1
	if index >= len(m.responses) {
		index = len(m.responses) - 1
	}
	return &types.ChatResponse{Content: m.responses[index]}, nil
}

func (m *quizChat) ChatStream(context.Context, []chat.Message, *chat.ChatOptions) (<-chan types.StreamResponse, error) {
	return nil, errors.New("not implemented")
}
func (m *quizChat) GetModelName() string { return "quiz-test-model" }
func (m *quizChat) GetModelID() string   { return "model-1" }

type quizModelService struct {
	interfaces.ModelService
	model chat.Chat
}

func (s *quizModelService) GetChatModel(context.Context, string) (chat.Chat, error) {
	return s.model, nil
}

type quizFixture struct {
	service *Service
	repo    interfaces.LearningRepository
	db      *gorm.DB
	model   *quizChat
	page    *types.WikiPage
	chunks  *quizChunkRepository
}

func newQuizFixture(t *testing.T, responses ...string) *quizFixture {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&types.LearningProfile{}, &types.LearningConceptIdentity{}, &types.UserConceptState{},
		&types.QuizItem{}, &types.LearningScan{}, &types.QuizAttempt{}, &types.LearningEvidence{},
	))
	repo := repository.NewLearningRepository(db)
	page := &types.WikiPage{
		ID: "page-rag", TenantID: 7, KnowledgeBaseID: "kb-a", Slug: "concept/rag", Title: "RAG",
		Summary:  "Retrieval-augmented generation grounds an answer in retrieved source passages.",
		PageType: types.WikiPageTypeConcept, Status: types.WikiPageStatusPublished,
		ChunkRefs: types.StringArray{"chunk-a", "chunk-b"},
	}
	chunks := &quizChunkRepository{chunks: map[string]*types.Chunk{
		"chunk-a": {ID: "chunk-a", TenantID: 7, KnowledgeBaseID: "kb-a", KnowledgeID: "knowledge-a", IsEnabled: true, ChunkType: types.ChunkTypeText,
			Content: "Retrieval-augmented generation first retrieves passages from an external knowledge source and provides those passages to the language model as context."},
		"chunk-b": {ID: "chunk-b", TenantID: 7, KnowledgeBaseID: "kb-a", KnowledgeID: "knowledge-a", IsEnabled: true, ChunkType: types.ChunkTypeText,
			Content: "Grounding the generated answer in retrieved passages can improve factual traceability because the answer can cite the source chunks that support it."},
	}}
	model := &quizChat{responses: responses}
	service := NewService(
		repo,
		&quizWikiRepository{pages: map[string]*types.WikiPage{"page-rag": page}},
		chunks,
		&exposureKBService{kbs: map[string]*types.KnowledgeBase{
			"kb-a": {ID: "kb-a", TenantID: 7, SummaryModelID: "model-1"},
		}},
		&quizModelService{model: model},
	).(*Service)
	require.NoError(t, repo.CreateConceptIdentity(context.Background(), &types.LearningConceptIdentity{
		ConceptKey: "concept-rag", TenantID: 7, KnowledgeBaseID: "kb-a", CurrentWikiPageID: ptr("page-rag"),
		Slug: page.Slug, Title: page.Title, LearningEligible: true, Status: types.LearningConceptIdentityActive,
	}))
	return &quizFixture{service: service, repo: repo, db: db, model: model, page: page, chunks: chunks}
}

func ptr(value string) *string { return &value }

func validQuizJSON(prefix string, count int) string {
	items := make([]map[string]any, 0, count)
	for i := 0; i < count; i++ {
		items = append(items, map[string]any{
			"question":         fmt.Sprintf("%s grounded question %d?", prefix, i+1),
			"options":          []string{"retrieved passages", "random weights", "only a UI theme", "an unrelated fact"},
			"correct_option":   0,
			"explanation":      "The cited chunk states that retrieved passages are supplied as context.",
			"source_chunk_ids": []string{"chunk-a"}, "difficulty": "medium",
		})
	}
	payload, _ := json.Marshal(map[string]any{"items": items})
	return string(payload)
}

func TestGenerateQuizItemsPersistsFourGroundedMCQsAndHidesAnswers(t *testing.T) {
	fixture := newQuizFixture(t, validQuizJSON("RAG", 4))
	result, err := fixture.service.GenerateQuizItems(learningWebContext(7, "alice"), "kb-a", "concept-rag")
	require.NoError(t, err)
	require.Equal(t, "ready", result.Status)
	require.Equal(t, 4, result.Generated)
	require.Equal(t, 1, result.AttemptCount)
	require.Len(t, result.Items, 4)
	require.Equal(t, 1, fixture.model.calls)

	var stored []types.QuizItem
	require.NoError(t, fixture.db.Order("question").Find(&stored).Error)
	require.Len(t, stored, 4)
	for _, item := range stored {
		require.Len(t, item.Options, 4)
		require.Equal(t, 0, item.CorrectOption)
		require.NotEmpty(t, item.Explanation)
		require.Equal(t, types.StringArray{"chunk-a"}, item.SourceChunkIDs)
		require.NotEmpty(t, item.QuestionHash)
		require.Equal(t, quizPromptVersion, item.PromptVersion)
	}

	wire, err := json.Marshal(result.Items[0])
	require.NoError(t, err)
	require.NotContains(t, string(wire), "correct_option")
	require.NotContains(t, string(wire), "explanation")

	again, err := fixture.service.GenerateQuizItems(learningWebContext(7, "alice"), "kb-a", "concept-rag")
	require.NoError(t, err)
	require.Zero(t, again.Generated)
	require.Equal(t, 1, fixture.model.calls, "a full lazy bank must not call the model again")
}

func TestGenerateQuizItemsRetriesMalformedOutputWithoutDirtyWrites(t *testing.T) {
	fixture := newQuizFixture(t, "not-json")
	_, err := fixture.service.GenerateQuizItems(learningWebContext(7, "alice"), "kb-a", "concept-rag")
	require.ErrorIs(t, err, ErrInvalidQuizOutput)
	require.Equal(t, quizGenerationTries, fixture.model.calls)
	var count int64
	require.NoError(t, fixture.db.Model(&types.QuizItem{}).Count(&count).Error)
	require.Zero(t, count)
}

func TestGenerateQuizItemsRetriesThenAcceptsCorrectedBatch(t *testing.T) {
	fixture := newQuizFixture(t, "bad", validQuizJSON("fixed", 4))
	result, err := fixture.service.GenerateQuizItems(learningWebContext(7, "alice"), "kb-a", "concept-rag")
	require.NoError(t, err)
	require.Equal(t, 2, result.AttemptCount)
	require.Len(t, result.Items, 4)
}

func TestQuizOutputDeterministicValidation(t *testing.T) {
	fixture := newQuizFixture(t, validQuizJSON("unused", 4))
	source, err := fixture.service.resolveQuizSource(learningWebContext(7, "alice"), "kb-a", "concept-rag")
	require.NoError(t, err)

	tests := []struct {
		name     string
		mutate   func(map[string]any)
		existing map[string]struct{}
	}{
		{name: "three options", mutate: func(item map[string]any) { item["options"] = []string{"a", "b", "c"} }},
		{name: "duplicate options", mutate: func(item map[string]any) { item["options"] = []string{"same", "same", "c", "d"} }},
		{name: "bad correct index", mutate: func(item map[string]any) { item["correct_option"] = 4 }},
		{name: "source override", mutate: func(item map[string]any) { item["source_chunk_ids"] = []string{"chunk-other"} }},
		{name: "duplicate question", existing: map[string]struct{}{hashQuizQuestion("question one?"): {}}, mutate: func(map[string]any) {}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			item := map[string]any{
				"question": "question one?", "options": []string{"a", "b", "c", "d"},
				"correct_option": 0, "explanation": "because source", "source_chunk_ids": []string{"chunk-a"},
			}
			test.mutate(item)
			payload, marshalErr := json.Marshal(map[string]any{"items": []any{item}})
			require.NoError(t, marshalErr)
			_, validationErr := validateQuizOutput(string(payload), 1, source, test.existing)
			require.Error(t, validationErr)
		})
	}
}

func TestGenerateQuizItemsRejectsConceptWithoutValidChunkRefs(t *testing.T) {
	fixture := newQuizFixture(t, validQuizJSON("unused", 4))
	fixture.page.ChunkRefs = nil
	_, err := fixture.service.GenerateQuizItems(learningWebContext(7, "alice"), "kb-a", "concept-rag")
	require.ErrorIs(t, err, ErrQuizCannotGenerate)
	require.Zero(t, fixture.model.calls)
}

func TestGenerateQuizItemsRejectsConceptFromAnotherKnowledgeBase(t *testing.T) {
	fixture := newQuizFixture(t, validQuizJSON("unused", 4))
	_, err := fixture.service.GenerateQuizItems(learningWebContext(7, "alice"), "kb-other", "concept-rag")
	require.ErrorIs(t, err, ErrLearningConceptNotFound)
}

func TestQuizItemsRequireSignedInWebPrincipal(t *testing.T) {
	fixture := newQuizFixture(t, validQuizJSON("unused", 4))
	ctx := context.WithValue(context.Background(), types.TenantIDContextKey, uint64(7))
	ctx = types.WithPrincipal(ctx, types.Principal{Type: types.PrincipalIMUser, ID: "other-user"})
	_, err := fixture.service.GetQuizItems(ctx, "kb-a", "concept-rag")
	require.ErrorIs(t, err, ErrUnsupportedPrincipal)
}

func TestChangedSourceMarksOldQuizBankStale(t *testing.T) {
	fixture := newQuizFixture(t, validQuizJSON("old", 4), validQuizJSON("new", 4))
	_, err := fixture.service.GenerateQuizItems(learningWebContext(7, "alice"), "kb-a", "concept-rag")
	require.NoError(t, err)
	fixture.chunks.chunks["chunk-a"].Content += " The source was materially revised."
	fixture.chunks.chunks["chunk-a"].ContentRevision++
	_, err = fixture.service.GenerateQuizItems(learningWebContext(7, "alice"), "kb-a", "concept-rag")
	require.NoError(t, err)
	var staleCount, currentCount int64
	require.NoError(t, fixture.db.Model(&types.QuizItem{}).Where("is_stale = ?", true).Count(&staleCount).Error)
	require.NoError(t, fixture.db.Model(&types.QuizItem{}).Where("is_stale = ?", false).Count(&currentCount).Error)
	require.Equal(t, int64(4), staleCount)
	require.Equal(t, int64(4), currentCount)
}
