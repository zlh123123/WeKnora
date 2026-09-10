package learning

import (
	"context"
	"errors"
	"sort"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// GetLearningOverlay returns one caller-scoped batch projection for the
// shared Wiki graph. Missing state is represented explicitly as unseen.
func (s *Service) GetLearningOverlay(
	ctx context.Context, knowledgeBaseID string,
) (*types.LearningOverlay, error) {
	callerScope, err := ResolveScope(ctx, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	if s.kbService == nil {
		return nil, errors.New("learning: knowledge base service is not configured")
	}
	kb, err := s.kbService.GetKnowledgeBaseByIDOnly(ctx, knowledgeBaseID)
	if err != nil || kb == nil || kb.TenantID == 0 {
		return nil, ErrLearningConceptNotFound
	}
	ownerCtx := context.WithValue(ctx, types.TenantIDContextKey, kb.TenantID)
	scope := interfaces.LearningScope{
		TenantID: kb.TenantID, SubjectID: callerScope.SubjectID, KnowledgeBaseID: knowledgeBaseID,
	}
	profile, err := s.repo.GetProfile(ownerCtx, scope)
	if err != nil {
		return nil, err
	}
	unlock := s.lockLearningKB(kb.TenantID, knowledgeBaseID)
	identities, err := s.reconcileConceptIdentities(ownerCtx, scope, kb)
	unlock()
	if err != nil {
		return nil, err
	}
	states, err := s.repo.ListConceptStates(ownerCtx, scope)
	if err != nil {
		return nil, err
	}
	stateByConcept := make(map[string]*types.UserConceptState, len(states))
	for _, state := range states {
		if state != nil {
			stateByConcept[state.ConceptKey] = state
		}
	}
	items := make([]types.LearningOverlayItem, 0, len(identities))
	for _, identity := range identities {
		if identity == nil || identity.CurrentWikiPageID == nil ||
			identity.Status == types.LearningConceptIdentityOrphaned {
			continue
		}
		item := types.LearningOverlayItem{
			ConceptKey: identity.ConceptKey, WikiPageID: *identity.CurrentWikiPageID,
			Slug: identity.Slug, Status: types.LearningProfileStatusUnseen,
		}
		if state := stateByConcept[identity.ConceptKey]; state != nil {
			item.Status = state.Status
			if item.Status == "" {
				item.Status = types.LearningProfileStatusUnseen
			}
			item.ExposureCount = state.ExposureCount
			item.ExposureWeight = state.ExposureWeight
			item.VerifiedMastery = state.VerifiedMastery
			item.MasteryConfidence = state.MasteryConfidence
			item.QuizAttemptCount = state.QuizAttemptCount
			item.LastExposedAt = state.LastExposedAt
			item.LastAssessedAt = state.LastAssessedAt
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Slug == items[j].Slug {
			return items[i].ConceptKey < items[j].ConceptKey
		}
		return items[i].Slug < items[j].Slug
	})
	return &types.LearningOverlay{TrackingEnabled: profile != nil && profile.TrackingEnabled, Items: items}, nil
}
