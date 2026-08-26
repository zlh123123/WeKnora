package learning

import (
	"context"
	"sort"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

// GetConceptInsights returns only caller-scoped, presentation-safe evidence.
// Recommendation ranking is deliberately deterministic and does not call an LLM.
func (s *Service) GetConceptInsights(ctx context.Context, knowledgeBaseID, conceptKey string) (*types.LearningConceptInsights, error) {
	scope, err := ResolveScope(ctx, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	conceptKey = strings.TrimSpace(conceptKey)
	if conceptKey == "" || s.kbService == nil {
		return nil, ErrLearningConceptNotFound
	}
	kb, err := s.kbService.GetKnowledgeBaseByIDOnly(ctx, knowledgeBaseID)
	if err != nil || kb == nil || kb.TenantID == 0 {
		return nil, ErrLearningConceptNotFound
	}
	ownerCtx := context.WithValue(ctx, types.TenantIDContextKey, kb.TenantID)
	identity, err := s.repo.GetConceptIdentity(ownerCtx, kb.TenantID, knowledgeBaseID, conceptKey)
	if err != nil || identity == nil || identity.Status == types.LearningConceptIdentityOrphaned {
		return nil, ErrLearningConceptNotFound
	}
	state, err := s.repo.GetConceptState(ownerCtx, scope, conceptKey)
	if err != nil {
		return nil, err
	}
	status := types.LearningProfileStatusUnseen
	if state != nil && state.Status != "" {
		status = state.Status
	}
	evidenceRows, err := s.repo.ListEvidence(ownerCtx, scope, conceptKey, 50)
	if err != nil {
		return nil, err
	}
	evidence := make([]types.LearningEvidenceView, 0, len(evidenceRows))
	for _, row := range evidenceRows {
		if row == nil {
			continue
		}
		evidence = append(evidence, types.LearningEvidenceView{
			ID: row.ID, EventType: row.EventType, Confidence: row.Confidence,
			EvidenceValue: row.EvidenceValue, OccurredAt: row.OccurredAt,
			SessionID: row.SessionID, MessageID: row.MessageID, ChunkID: row.ChunkID,
			QuizItemID: row.QuizItemID, QuizAttemptID: row.QuizAttemptID,
		})
	}
	insights := &types.LearningConceptInsights{ConceptKey: conceptKey, Status: status, Evidence: evidence}
	if state != nil && ((status == types.LearningProfileStatusVerifiedWeak && (state.ExposureCount >= 2 || state.ExposureWeight >= 1.5)) ||
		(status == types.LearningProfileStatusUncertain && state.ExposureCount >= 3 && state.QuizAttemptCount >= 2)) {
		insights.IsKnowledgeGap = true
		if status == types.LearningProfileStatusVerifiedWeak {
			insights.GapReason = "已多次接触，但最近掌握验证较弱。"
		} else {
			insights.GapReason = "已多次接触，但掌握程度仍未确认。"
		}
	}
	insights.Recommendation = s.recommendation(ownerCtx, scope, conceptKey)
	return insights, nil
}

func (s *Service) recommendation(ctx context.Context, scope interfaces.LearningScope, current string) *types.LearningRecommendation {
	identities, err := s.repo.ListConceptIdentities(ctx, scope.TenantID, scope.KnowledgeBaseID)
	if err != nil {
		return nil
	}
	states, err := s.repo.ListConceptStates(ctx, scope)
	if err != nil {
		return nil
	}
	stateByKey := make(map[string]*types.UserConceptState, len(states))
	for _, state := range states {
		if state != nil {
			stateByKey[state.ConceptKey] = state
		}
	}
	candidates := make([]*types.LearningConceptIdentity, 0, len(identities))
	for _, identity := range identities {
		if identity == nil || identity.ConceptKey == current || identity.CurrentWikiPageID == nil ||
			!identity.LearningEligible || identity.Status != types.LearningConceptIdentityActive {
			continue
		}
		if state := stateByKey[identity.ConceptKey]; state != nil &&
			(state.Status == types.LearningProfileStatusVerifiedWeak || state.Status == types.LearningProfileStatusUncertain || state.Status == types.LearningProfileStatusExposed) {
			candidates = append(candidates, identity)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left, right := stateByKey[candidates[i].ConceptKey], stateByKey[candidates[j].ConceptKey]
		priority := func(state *types.UserConceptState) int {
			if state == nil {
				return 0
			}
			switch state.Status {
			case types.LearningProfileStatusVerifiedWeak:
				return 3
			case types.LearningProfileStatusUncertain:
				return 2
			case types.LearningProfileStatusExposed:
				return 1
			default:
				return 0
			}
		}
		lp, rp := priority(left), priority(right)
		if lp != rp {
			return lp > rp
		}
		le, re := 0, 0
		if left != nil {
			le = left.ExposureCount
		}
		if right != nil {
			re = right.ExposureCount
		}
		if le != re {
			return le > re
		}
		return candidates[i].ConceptKey < candidates[j].ConceptKey
	})
	if len(candidates) == 0 {
		return nil
	}
	selected := candidates[0]
	state := stateByKey[selected.ConceptKey]
	reason := "建议继续巩固该知识点。"
	if state != nil && state.Status == types.LearningProfileStatusVerifiedWeak {
		reason = "该知识点的掌握验证较弱，建议优先巩固。"
	} else if state != nil && state.Status == types.LearningProfileStatusUncertain {
		reason = "该知识点已接触但掌握仍未确认，建议再次验证。"
	}
	return &types.LearningRecommendation{ConceptKey: selected.ConceptKey, Title: selected.Title, Slug: selected.Slug, Reason: reason}
}

var _ interfaces.LearningService = (*Service)(nil)
