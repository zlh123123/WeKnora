// Package learning implements the caller-scoped Knowledge MRI profile layer.
package learning

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

var (
	ErrNoLearningScope      = errors.New("learning: no caller scope")
	ErrUnsupportedPrincipal = errors.New("learning: only signed-in web users are eligible")
	ErrInvalidKnowledgeBase = errors.New("learning: knowledge base id is required")
)

type Service struct {
	scanLocks    [64]sync.Mutex
	repo         interfaces.LearningRepository
	wikiRepo     interfaces.WikiPageRepository
	chunkRepo    interfaces.ChunkRepository
	kbService    interfaces.KnowledgeBaseService
	modelService interfaces.ModelService
}

func NewService(
	repo interfaces.LearningRepository,
	wikiRepo interfaces.WikiPageRepository,
	chunkRepo interfaces.ChunkRepository,
	kbService interfaces.KnowledgeBaseService,
	modelService interfaces.ModelService,
) interfaces.LearningService {
	return &Service{repo: repo, wikiRepo: wikiRepo, chunkRepo: chunkRepo, kbService: kbService, modelService: modelService}
}

// ResolveScope derives subject ownership from Principal.StorageID(). Learning
// is deliberately narrower than Memory: only signed-in Web users participate
// in the MVP, even though other principal kinds also have stable storage IDs.
func ResolveScope(ctx context.Context, knowledgeBaseID string) (interfaces.LearningScope, error) {
	knowledgeBaseID = strings.TrimSpace(knowledgeBaseID)
	if knowledgeBaseID == "" {
		return interfaces.LearningScope{}, ErrInvalidKnowledgeBase
	}
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		return interfaces.LearningScope{}, ErrNoLearningScope
	}
	principal, ok := types.PrincipalFromContext(ctx)
	if !ok {
		return interfaces.LearningScope{}, ErrNoLearningScope
	}
	if principal.Type != types.PrincipalWebUser {
		return interfaces.LearningScope{}, ErrUnsupportedPrincipal
	}
	subjectID := principal.StorageID()
	if subjectID == "" {
		return interfaces.LearningScope{}, ErrNoLearningScope
	}
	return interfaces.LearningScope{
		TenantID: tenantID, SubjectID: subjectID, KnowledgeBaseID: knowledgeBaseID,
	}, nil
}

func (s *Service) GetProfile(
	ctx context.Context, knowledgeBaseID string,
) (*types.LearningProfile, error) {
	scope, err := ResolveScope(ctx, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	return s.repo.EnsureProfile(ctx, scope)
}

func (s *Service) SetTracking(
	ctx context.Context, knowledgeBaseID string, enabled bool,
) (*types.LearningProfile, error) {
	scope, err := ResolveScope(ctx, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	return s.repo.SetTracking(ctx, scope, enabled)
}

func (s *Service) Clear(
	ctx context.Context, knowledgeBaseID string,
) (*types.LearningProfile, error) {
	scope, err := ResolveScope(ctx, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	return s.repo.ClearScope(ctx, scope)
}

func (s *Service) Export(ctx context.Context, knowledgeBaseID string) (*types.LearningExport, error) {
	scope, err := ResolveScope(ctx, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	return s.repo.ExportScope(ctx, scope)
}

func (s *Service) ListConceptStates(
	ctx context.Context, knowledgeBaseID string,
) ([]*types.UserConceptState, error) {
	scope, err := ResolveScope(ctx, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListConceptStates(ctx, scope)
}

func (s *Service) EnsureConceptIdentity(
	ctx context.Context,
	knowledgeBaseID string,
	lookup interfaces.LearningConceptLookup,
) (*types.LearningConceptIdentity, interfaces.LearningConceptResolution, error) {
	knowledgeBaseID = strings.TrimSpace(knowledgeBaseID)
	if knowledgeBaseID == "" {
		return nil, interfaces.LearningConceptResolution{}, ErrInvalidKnowledgeBase
	}
	tenantID, ok := types.TenantIDFromContext(ctx)
	if !ok || tenantID == 0 {
		return nil, interfaces.LearningConceptResolution{}, ErrNoLearningScope
	}
	resolution, err := s.repo.ResolveConceptIdentity(ctx, tenantID, knowledgeBaseID, lookup)
	if err != nil {
		return nil, resolution, err
	}
	if resolution.Identity != nil {
		identity := resolution.Identity
		if pageID := strings.TrimSpace(lookup.CurrentWikiPageID); pageID != "" {
			identity.CurrentWikiPageID = &pageID
		}
		identity.Slug = strings.TrimSpace(lookup.Slug)
		identity.Title = strings.TrimSpace(lookup.Title)
		identity.Aliases = append(types.StringArray(nil), lookup.Aliases...)
		identity.Status = types.LearningConceptIdentityActive
		if err := s.repo.UpdateConceptIdentity(ctx, identity); err != nil {
			return nil, resolution, err
		}
		return identity, resolution, nil
	}
	identity := &types.LearningConceptIdentity{
		TenantID:         tenantID,
		KnowledgeBaseID:  knowledgeBaseID,
		Slug:             lookup.Slug,
		Title:            lookup.Title,
		Aliases:          append(types.StringArray(nil), lookup.Aliases...),
		LearningEligible: true,
		Status:           types.LearningConceptIdentityActive,
	}
	if pageID := strings.TrimSpace(lookup.CurrentWikiPageID); pageID != "" {
		identity.CurrentWikiPageID = &pageID
	}
	if resolution.Ambiguous {
		identity.Status = types.LearningConceptIdentityOrphaned
	}
	if err := s.repo.CreateConceptIdentity(ctx, identity); err != nil {
		return nil, resolution, err
	}
	return identity, resolution, nil
}

type displayedReference struct {
	chunkID     string
	knowledgeID string
}

// RecordDisplayedReferences projects the references on one persisted final
// assistant message into caller-scoped exposure evidence. It intentionally
// ignores raw retrieval context and never writes mastery fields.
func (s *Service) RecordDisplayedReferences(ctx context.Context, message *types.Message) error {
	if message == nil || message.Role != "assistant" || !message.IsCompleted ||
		strings.TrimSpace(message.ID) == "" || len(message.KnowledgeReferences) == 0 {
		return nil
	}
	if s.wikiRepo == nil || s.chunkRepo == nil || s.kbService == nil {
		return errors.New("learning: exposure dependencies are not configured")
	}
	principal, ok := types.PrincipalFromContext(ctx)
	if !ok {
		return ErrNoLearningScope
	}
	if principal.Type != types.PrincipalWebUser {
		return ErrUnsupportedPrincipal
	}

	byKB := make(map[string]map[string]displayedReference)
	for _, ref := range message.KnowledgeReferences {
		if ref == nil {
			continue
		}
		kbID := strings.TrimSpace(ref.KnowledgeBaseID)
		chunkID := strings.TrimSpace(ref.ID)
		if kbID == "" || chunkID == "" {
			continue
		}
		if byKB[kbID] == nil {
			byKB[kbID] = make(map[string]displayedReference)
		}
		byKB[kbID][chunkID] = displayedReference{chunkID: chunkID, knowledgeID: strings.TrimSpace(ref.KnowledgeID)}
	}
	if len(byKB) == 0 {
		return nil
	}

	occurredAt := message.UpdatedAt
	if occurredAt.IsZero() {
		occurredAt = message.CreatedAt
	}
	if occurredAt.IsZero() {
		occurredAt = time.Now()
	}

	kbIDs := make([]string, 0, len(byKB))
	for kbID := range byKB {
		kbIDs = append(kbIDs, kbID)
	}
	sort.Strings(kbIDs)
	var joinedErr error
	for _, kbID := range kbIDs {
		if err := s.recordDisplayedReferencesForKB(ctx, message, kbID, byKB[kbID], occurredAt); err != nil {
			joinedErr = errors.Join(joinedErr, err)
		}
	}
	return joinedErr
}

func (s *Service) recordDisplayedReferencesForKB(
	ctx context.Context,
	message *types.Message,
	kbID string,
	references map[string]displayedReference,
	occurredAt time.Time,
) error {
	kb, err := s.kbService.GetKnowledgeBaseByIDOnly(ctx, kbID)
	if err != nil {
		return fmt.Errorf("learning: resolve knowledge base %s: %w", kbID, err)
	}
	if kb == nil || kb.TenantID == 0 {
		return fmt.Errorf("learning: knowledge base %s has no owner tenant", kbID)
	}
	ownerCtx := context.WithValue(ctx, types.TenantIDContextKey, kb.TenantID)
	scope, err := ResolveScope(ownerCtx, kbID)
	if err != nil {
		return err
	}
	profile, err := s.repo.GetProfile(ownerCtx, scope)
	if err != nil {
		return err
	}
	if profile == nil || !profile.TrackingEnabled {
		return nil
	}
	if profile.EnabledAt != nil && occurredAt.Before(*profile.EnabledAt) {
		return nil
	}
	if profile.ClearedAt != nil && !occurredAt.After(*profile.ClearedAt) {
		return nil
	}

	chunkIDs := make([]string, 0, len(references))
	for chunkID := range references {
		chunkIDs = append(chunkIDs, chunkID)
	}
	sort.Strings(chunkIDs)
	chunks, err := s.chunkRepo.ListChunksByIDOnly(ownerCtx, chunkIDs)
	if err != nil {
		return fmt.Errorf("learning: load displayed chunks: %w", err)
	}
	validChunks := make(map[string]*types.Chunk, len(chunks))
	for _, chunk := range chunks {
		if chunk == nil || chunk.KnowledgeBaseID != kbID {
			continue
		}
		ref := references[chunk.ID]
		if ref.knowledgeID != "" && chunk.KnowledgeID != ref.knowledgeID {
			continue
		}
		validChunks[chunk.ID] = chunk
	}
	if len(validChunks) == 0 {
		return nil
	}

	pages, err := s.wikiRepo.ListByType(ownerCtx, kbID, types.WikiPageTypeConcept)
	if err != nil {
		return fmt.Errorf("learning: load concept pages: %w", err)
	}
	candidates := make(map[string][]*types.WikiPage, len(validChunks))
	for _, page := range pages {
		if page == nil || page.KnowledgeBaseID != kbID || page.Status == types.WikiPageStatusArchived {
			continue
		}
		seenChunk := make(map[string]struct{}, len(page.ChunkRefs))
		for _, chunkID := range page.ChunkRefs {
			chunkID = strings.TrimSpace(chunkID)
			if _, ok := validChunks[chunkID]; !ok {
				continue
			}
			if _, duplicate := seenChunk[chunkID]; duplicate {
				continue
			}
			seenChunk[chunkID] = struct{}{}
			candidates[chunkID] = append(candidates[chunkID], page)
		}
	}

	var joinedErr error
	for _, chunkID := range chunkIDs {
		pagesForChunk := candidates[chunkID]
		if len(pagesForChunk) == 0 {
			continue
		}
		sort.Slice(pagesForChunk, func(i, j int) bool { return pagesForChunk[i].ID < pagesForChunk[j].ID })
		confidence := 1 / float64(len(pagesForChunk))
		for _, page := range pagesForChunk {
			identity, _, err := s.EnsureConceptIdentity(ownerCtx, kbID, interfaces.LearningConceptLookup{
				CurrentWikiPageID: page.ID,
				Slug:              page.Slug,
				Title:             page.Title,
				Aliases:           append([]string(nil), page.Aliases...),
			})
			if err != nil {
				joinedErr = errors.Join(joinedErr, fmt.Errorf("learning: resolve concept page %s: %w", page.ID, err))
				continue
			}
			messageID, sessionID := message.ID, message.SessionID
			knowledgeID := references[chunkID].knowledgeID
			wikiPageID, evidenceChunkID := page.ID, chunkID
			evidence := &types.LearningEvidence{
				ConceptKey:     identity.ConceptKey,
				WikiPageID:     &wikiPageID,
				SessionID:      &sessionID,
				MessageID:      &messageID,
				KnowledgeID:    &knowledgeID,
				ChunkID:        &evidenceChunkID,
				EventType:      types.LearningEvidenceDisplayedReference,
				EvidenceValue:  1,
				Confidence:     confidence,
				OccurredAt:     occurredAt,
				IdempotencyKey: displayedReferenceKey(scope, identity.ConceptKey, message.ID, chunkID),
				Metadata: types.JSONMap{
					"candidate_count": len(pagesForChunk),
					"source":          "assistant_message",
				},
			}
			if _, err := s.repo.AppendExposureEvidence(ownerCtx, scope, evidence); err != nil {
				joinedErr = errors.Join(joinedErr, fmt.Errorf("learning: append exposure: %w", err))
			}
		}
	}
	return joinedErr
}

func displayedReferenceKey(
	scope interfaces.LearningScope, conceptKey, messageID, chunkID string,
) string {
	raw := strings.Join([]string{
		scope.SubjectID, scope.KnowledgeBaseID, conceptKey, messageID,
		types.LearningEvidenceDisplayedReference, chunkID,
	}, "\x00")
	return fmt.Sprintf("displayed_reference:%x", sha256.Sum256([]byte(raw)))
}
