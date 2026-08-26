package learning

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Tencent/WeKnora/internal/models/chat"
	"github.com/Tencent/WeKnora/internal/types"
)

const (
	quizBankTarget      = 4
	quizGenerationTries = 3
	quizPromptVersion   = "knowledge-mri-mcq-v1"
	quizMaxContextRunes = 14000
	quizMaxChunkRunes   = 5000
	quizMinContextRunes = 80
)

var (
	ErrLearningConceptNotFound = errors.New("learning: concept not found in knowledge base")
	ErrQuizCannotGenerate      = errors.New("learning: cannot_generate")
	ErrInvalidQuizOutput       = errors.New("learning: invalid quiz model output")
)

var quizOutputSchema = json.RawMessage(`{
  "type":"object",
  "properties":{"items":{"type":"array","items":{"type":"object","properties":{
    "question":{"type":"string"},
    "options":{"type":"array","minItems":4,"maxItems":4,"items":{"type":"string"}},
    "correct_option":{"type":"integer","minimum":0,"maximum":3},
    "explanation":{"type":"string"},
    "source_chunk_ids":{"type":"array","minItems":1,"items":{"type":"string"}},
    "difficulty":{"type":"string"}
  },"required":["question","options","correct_option","explanation","source_chunk_ids"]}}},
  "required":["items"]
}`)

type quizModelOutput struct {
	Items []quizModelItem `json:"items"`
}

type quizModelItem struct {
	Question       string   `json:"question"`
	Options        []string `json:"options"`
	CorrectOption  int      `json:"correct_option"`
	Explanation    string   `json:"explanation"`
	SourceChunkIDs []string `json:"source_chunk_ids"`
	Difficulty     string   `json:"difficulty"`
}

type quizSource struct {
	page       *types.WikiPage
	identity   *types.LearningConceptIdentity
	chunks     []*types.Chunk
	sourceHash string
	ownerCtx   context.Context
	kb         *types.KnowledgeBase
}

func (s *Service) GetQuizItems(
	ctx context.Context, knowledgeBaseID, conceptKey string,
) (*types.QuizBankResult, error) {
	source, err := s.resolveQuizSource(ctx, knowledgeBaseID, conceptKey)
	if err != nil {
		return nil, err
	}
	items, err := s.repo.ListQuizItems(source.ownerCtx, source.kb.TenantID, knowledgeBaseID, conceptKey, source.sourceHash)
	if err != nil {
		return nil, err
	}
	status := "ready"
	if len(items) == 0 {
		status = "empty"
	}
	return quizBankResult(status, items, 0, 0, source.sourceHash), nil
}

func (s *Service) GenerateQuizItems(
	ctx context.Context, knowledgeBaseID, conceptKey string,
) (*types.QuizBankResult, error) {
	source, err := s.resolveQuizSource(ctx, knowledgeBaseID, conceptKey)
	if err != nil {
		return nil, err
	}
	existing, err := s.repo.ListQuizItems(source.ownerCtx, source.kb.TenantID, knowledgeBaseID, conceptKey, source.sourceHash)
	if err != nil {
		return nil, err
	}
	if len(existing) >= quizBankTarget {
		return quizBankResult("ready", existing[:quizBankTarget], 0, 0, source.sourceHash), nil
	}
	if s.modelService == nil || strings.TrimSpace(source.kb.SummaryModelID) == "" {
		return nil, fmt.Errorf("%w: summary model is not configured", ErrQuizCannotGenerate)
	}
	model, err := s.modelService.GetChatModel(source.ownerCtx, source.kb.SummaryModelID)
	if err != nil {
		return nil, fmt.Errorf("%w: load summary model: %v", ErrQuizCannotGenerate, err)
	}
	needed := quizBankTarget - len(existing)
	allExisting, err := s.repo.ListQuizItems(source.ownerCtx, source.kb.TenantID, knowledgeBaseID, conceptKey, "")
	if err != nil {
		return nil, err
	}
	existingHashes := make(map[string]struct{}, len(allExisting))
	for _, item := range allExisting {
		existingHashes[item.QuestionHash] = struct{}{}
	}

	systemPrompt, userPrompt := buildQuizPrompt(source, needed)
	var lastErr error
	for attempt := 1; attempt <= quizGenerationTries; attempt++ {
		prompt := userPrompt
		if lastErr != nil {
			prompt += "\n\nThe previous response was rejected by deterministic validation: " + lastErr.Error() +
				". Return a completely corrected JSON object only."
		}
		thinking := false
		response, callErr := model.Chat(source.ownerCtx, []chat.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		}, &chat.ChatOptions{
			Temperature: 0.2, MaxCompletionTokens: 1800, Thinking: &thinking, Format: quizOutputSchema,
		})
		if callErr != nil {
			lastErr = fmt.Errorf("model call failed: %w", callErr)
			continue
		}
		generated, validationErr := validateQuizOutput(response.Content, needed, source, existingHashes)
		if validationErr != nil {
			lastErr = validationErr
			continue
		}
		modelInfo := types.JSONMap{
			"model_id": model.GetModelID(), "model_name": model.GetModelName(),
			"attempt": attempt, "prompt_tokens": response.Usage.PromptTokens,
			"completion_tokens": response.Usage.CompletionTokens,
		}
		for _, item := range generated {
			item.ModelInfo = modelInfo
		}
		saved, saveErr := s.repo.SaveQuizItems(
			source.ownerCtx, source.kb.TenantID, knowledgeBaseID, conceptKey, source.sourceHash, generated,
		)
		if saveErr != nil {
			return nil, saveErr
		}
		generatedCount := len(saved) - len(existing)
		if generatedCount < 0 {
			generatedCount = 0
		}
		return quizBankResult("ready", saved, generatedCount, attempt, source.sourceHash), nil
	}
	return nil, fmt.Errorf("%w after %d attempts: %v", ErrInvalidQuizOutput, quizGenerationTries, lastErr)
}

func (s *Service) resolveQuizSource(
	ctx context.Context, knowledgeBaseID, conceptKey string,
) (*quizSource, error) {
	if _, err := ResolveScope(ctx, knowledgeBaseID); err != nil {
		return nil, err
	}
	conceptKey = strings.TrimSpace(conceptKey)
	if conceptKey == "" {
		return nil, ErrLearningConceptNotFound
	}
	if s.wikiRepo == nil || s.chunkRepo == nil || s.kbService == nil {
		return nil, errors.New("learning: quiz dependencies are not configured")
	}
	kb, err := s.kbService.GetKnowledgeBaseByIDOnly(ctx, knowledgeBaseID)
	if err != nil || kb == nil || kb.TenantID == 0 {
		return nil, ErrLearningConceptNotFound
	}
	ownerCtx := context.WithValue(ctx, types.TenantIDContextKey, kb.TenantID)
	identity, err := s.repo.GetConceptIdentity(ownerCtx, kb.TenantID, knowledgeBaseID, conceptKey)
	if err != nil {
		return nil, err
	}
	if identity == nil || identity.CurrentWikiPageID == nil || !identity.LearningEligible ||
		identity.Status != types.LearningConceptIdentityActive {
		return nil, ErrLearningConceptNotFound
	}
	page, err := s.wikiRepo.GetByID(ownerCtx, *identity.CurrentWikiPageID)
	if err != nil || page == nil || page.TenantID != kb.TenantID || page.KnowledgeBaseID != knowledgeBaseID ||
		page.PageType != types.WikiPageTypeConcept || page.Status != types.WikiPageStatusPublished {
		return nil, ErrLearningConceptNotFound
	}
	chunks, err := selectQuizChunks(ownerCtx, s.chunkRepo, kb, page)
	if err != nil {
		return nil, err
	}
	return &quizSource{
		page: page, identity: identity, chunks: chunks, sourceHash: hashQuizSource(chunks), ownerCtx: ownerCtx, kb: kb,
	}, nil
}

func selectQuizChunks(
	ctx context.Context, repo interface {
		ListChunksByIDOnly(context.Context, []string) ([]*types.Chunk, error)
	}, kb *types.KnowledgeBase, page *types.WikiPage,
) ([]*types.Chunk, error) {
	seen := make(map[string]struct{}, len(page.ChunkRefs))
	ids := make([]string, 0, len(page.ChunkRefs))
	for _, rawID := range page.ChunkRefs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: concept has no chunk references", ErrQuizCannotGenerate)
	}
	loaded, err := repo.ListChunksByIDOnly(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("%w: load chunks: %v", ErrQuizCannotGenerate, err)
	}
	valid := make([]*types.Chunk, 0, len(loaded))
	for _, chunk := range loaded {
		if chunk == nil || chunk.TenantID != kb.TenantID || chunk.KnowledgeBaseID != kb.ID ||
			!chunk.IsEnabled || (chunk.ChunkType != "" && chunk.ChunkType != types.ChunkTypeText) || strings.TrimSpace(chunk.Content) == "" {
			continue
		}
		valid = append(valid, chunk)
	}
	// ChunkRefs are already direct citations. Prefer shorter citations so the
	// bounded prompt carries several independent pieces of evidence.
	sort.Slice(valid, func(i, j int) bool {
		li, lj := utf8.RuneCountInString(valid[i].Content), utf8.RuneCountInString(valid[j].Content)
		if li == lj {
			return valid[i].ID < valid[j].ID
		}
		return li < lj
	})
	selected := make([]*types.Chunk, 0, 4)
	total := 0
	for _, chunk := range valid {
		if len(selected) == 4 || total >= quizMaxContextRunes {
			break
		}
		remaining := quizMaxContextRunes - total
		length := utf8.RuneCountInString(strings.TrimSpace(chunk.Content))
		if length > quizMaxChunkRunes {
			length = quizMaxChunkRunes
		}
		if length > remaining && len(selected) > 0 {
			continue
		}
		selected = append(selected, chunk)
		total += min(length, remaining)
	}
	if len(selected) == 0 || total < quizMinContextRunes {
		return nil, fmt.Errorf("%w: referenced chunk content is insufficient", ErrQuizCannotGenerate)
	}
	return selected, nil
}

func buildQuizPrompt(source *quizSource, count int) (string, string) {
	system := "You generate evidence-bound multiple-choice questions for Knowledge MRI. " +
		"Use only the supplied source chunks. Never use outside facts as the correct answer. " +
		"Return one JSON object only. Every question must have exactly four distinct options and exactly one correct option. " +
		"correct_option is zero-based (0..3). explanation must state why the answer follows from the cited chunks. " +
		"Each source_chunk_ids value must be copied from the supplied chunk IDs."
	var builder strings.Builder
	fmt.Fprintf(&builder, "Generate exactly %d questions.\nConcept title: %s\nConcept summary: %s\n", count, source.page.Title, source.page.Summary)
	for _, chunk := range source.chunks {
		fmt.Fprintf(&builder, "\n<SOURCE_CHUNK id=\"%s\">\n%s\n</SOURCE_CHUNK>\n", chunk.ID, truncateQuizRunes(strings.TrimSpace(chunk.Content), quizMaxChunkRunes))
	}
	return system, builder.String()
}

func validateQuizOutput(
	raw string, expected int, source *quizSource, existingHashes map[string]struct{},
) ([]*types.QuizItem, error) {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "```") && strings.HasSuffix(raw, "```") {
		raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(raw, "```json"), "```"))
		if strings.HasPrefix(raw, "```") {
			raw = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(raw, "```"), "```"))
		}
	}
	var output quizModelOutput
	if err := json.Unmarshal([]byte(raw), &output); err != nil {
		return nil, fmt.Errorf("malformed JSON: %w", err)
	}
	if len(output.Items) != expected {
		return nil, fmt.Errorf("item count is %d, want %d", len(output.Items), expected)
	}
	allowedSources := make(map[string]struct{}, len(source.chunks))
	for _, chunk := range source.chunks {
		allowedSources[chunk.ID] = struct{}{}
	}
	seenQuestions := make(map[string]struct{}, expected)
	items := make([]*types.QuizItem, 0, expected)
	for index, candidate := range output.Items {
		question := strings.TrimSpace(candidate.Question)
		explanation := strings.TrimSpace(candidate.Explanation)
		if question == "" || explanation == "" {
			return nil, fmt.Errorf("item %d has empty question or explanation", index)
		}
		if len(candidate.Options) != 4 {
			return nil, fmt.Errorf("item %d has %d options, want 4", index, len(candidate.Options))
		}
		options := make(types.StringArray, 4)
		seenOptions := make(map[string]struct{}, 4)
		for optionIndex, option := range candidate.Options {
			option = strings.TrimSpace(option)
			normalized := normalizeQuizText(option)
			if normalized == "" {
				return nil, fmt.Errorf("item %d option %d is empty", index, optionIndex)
			}
			if _, duplicate := seenOptions[normalized]; duplicate {
				return nil, fmt.Errorf("item %d has duplicate options", index)
			}
			seenOptions[normalized] = struct{}{}
			options[optionIndex] = option
		}
		if candidate.CorrectOption < 0 || candidate.CorrectOption >= 4 {
			return nil, fmt.Errorf("item %d correct_option is outside 0..3", index)
		}
		if len(candidate.SourceChunkIDs) == 0 {
			return nil, fmt.Errorf("item %d has no source chunks", index)
		}
		sourceIDs := make(types.StringArray, 0, len(candidate.SourceChunkIDs))
		seenSources := make(map[string]struct{}, len(candidate.SourceChunkIDs))
		for _, rawID := range candidate.SourceChunkIDs {
			id := strings.TrimSpace(rawID)
			if _, allowed := allowedSources[id]; !allowed {
				return nil, fmt.Errorf("item %d cites source outside input: %s", index, id)
			}
			if _, duplicate := seenSources[id]; duplicate {
				continue
			}
			seenSources[id] = struct{}{}
			sourceIDs = append(sourceIDs, id)
		}
		questionHash := hashQuizQuestion(question)
		if _, duplicate := existingHashes[questionHash]; duplicate {
			return nil, fmt.Errorf("item %d duplicates an existing question", index)
		}
		if _, duplicate := seenQuestions[questionHash]; duplicate {
			return nil, fmt.Errorf("item %d duplicates another generated question", index)
		}
		seenQuestions[questionHash] = struct{}{}
		difficulty := strings.ToLower(strings.TrimSpace(candidate.Difficulty))
		if difficulty == "" {
			difficulty = "medium"
		}
		items = append(items, &types.QuizItem{
			WikiPageID: &source.page.ID, Question: question, QuestionHash: questionHash,
			Options: options, CorrectOption: candidate.CorrectOption, Explanation: explanation,
			SourceChunkIDs: sourceIDs, SourceHash: source.sourceHash,
			PromptVersion: quizPromptVersion, Difficulty: difficulty, IsActive: true,
		})
	}
	return items, nil
}

func hashQuizSource(chunks []*types.Chunk) string {
	hash := sha256.New()
	for _, chunk := range chunks {
		_, _ = fmt.Fprintf(hash, "%s\x00%d\x00%s\x00", chunk.ID, chunk.ContentRevision, strings.TrimSpace(chunk.Content))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func hashQuizQuestion(question string) string {
	sum := sha256.Sum256([]byte(normalizeQuizText(question)))
	return hex.EncodeToString(sum[:])
}

func normalizeQuizText(value string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(value)) {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func truncateQuizRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}

func quizBankResult(
	status string, items []*types.QuizItem, generated, attempts int, sourceHash string,
) *types.QuizBankResult {
	views := make([]types.QuizItemView, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		views = append(views, types.QuizItemView{
			ID: item.ID, ConceptKey: item.ConceptKey, WikiPageID: item.WikiPageID,
			Question: item.Question, Options: append(types.StringArray(nil), item.Options...),
			SourceChunkIDs: append(types.StringArray(nil), item.SourceChunkIDs...),
			SourceHash:     item.SourceHash, PromptVersion: item.PromptVersion,
			Difficulty: item.Difficulty, CreatedAt: item.CreatedAt,
		})
	}
	return &types.QuizBankResult{
		Status: status, Items: views, Generated: generated, AttemptCount: attempts, SourceHash: sourceHash,
	}
}
