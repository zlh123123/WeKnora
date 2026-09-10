package learning

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
)

const (
	scanConceptTarget   = 3
	scanItemsPerConcept = 2
)

type scanCandidate struct {
	identity *types.LearningConceptIdentity
	source   *quizSource
	state    *types.UserConceptState
	degree   int
	priority float64
}

// StartOrResumeScan returns the caller's in-progress scan, or creates a new
// deterministic six-question scan. A concept that cannot yield two safe
// questions is skipped so one model failure never breaks the whole scan.
func (s *Service) StartOrResumeScan(ctx context.Context, knowledgeBaseID string) (*types.LearningScanView, error) {
	return s.startOrResumeScan(ctx, knowledgeBaseID, "")
}

func (s *Service) StartOrResumeConceptScan(ctx context.Context, knowledgeBaseID, conceptKey string) (*types.LearningScanView, error) {
	conceptKey = strings.TrimSpace(conceptKey)
	if conceptKey == "" {
		return nil, ErrLearningConceptNotFound
	}
	return s.startOrResumeScan(ctx, knowledgeBaseID, conceptKey)
}

func (s *Service) startOrResumeScan(ctx context.Context, knowledgeBaseID, conceptKey string) (*types.LearningScanView, error) {
	scope, ownerCtx, kb, err := s.scanScope(ctx, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	unlock := s.lockLearningKB(scope.TenantID, knowledgeBaseID)
	defer unlock()
	profile, err := s.repo.GetProfile(ownerCtx, scope)
	if err != nil {
		return nil, err
	}
	if profile == nil || !profile.TrackingEnabled {
		return nil, ErrLearningTrackingDisabled
	}
	if active, err := s.repo.GetActiveScan(ownerCtx, scope, conceptKey); err != nil {
		return nil, err
	} else if active != nil {
		return s.scanView(ownerCtx, scope, kb, active)
	}

	var candidates []scanCandidate
	target := scanConceptTarget * scanItemsPerConcept
	if conceptKey == "" {
		candidates, err = s.scanCandidates(ownerCtx, scope, kb)
	} else {
		var source *quizSource
		source, err = s.resolveQuizSource(ownerCtx, knowledgeBaseID, conceptKey)
		if err == nil {
			candidates = []scanCandidate{{identity: source.identity, source: source}}
		}
		target = scanItemsPerConcept
	}
	if err != nil {
		return nil, err
	}
	itemIDs := make(types.StringArray, 0, target)
	for _, candidate := range candidates {
		items, itemErr := s.scanItems(ownerCtx, scope, knowledgeBaseID, candidate)
		if itemErr != nil || len(items) < scanItemsPerConcept {
			continue
		}
		for _, item := range items[:scanItemsPerConcept] {
			itemIDs = append(itemIDs, item.ID)
		}
		if len(itemIDs) == target {
			break
		}
	}
	if len(itemIDs) != target {
		return nil, ErrQuizCannotGenerate
	}
	scan := &types.LearningScan{ID: uuid.NewString(), Status: types.LearningScanStatusPending, QuizItemIDs: itemIDs}
	if err := s.repo.CreateScan(ownerCtx, scope, scan); err != nil {
		return nil, err
	}
	return s.scanView(ownerCtx, scope, kb, scan)
}

func (s *Service) GetActiveScan(ctx context.Context, knowledgeBaseID string) (*types.LearningScanView, error) {
	scope, ownerCtx, kb, err := s.scanScope(ctx, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	scan, err := s.repo.GetActiveScan(ownerCtx, scope)
	if err != nil || scan == nil {
		return nil, err
	}
	return s.scanView(ownerCtx, scope, kb, scan)
}

func (s *Service) CompleteScan(ctx context.Context, knowledgeBaseID, scanID string) (*types.LearningScanView, error) {
	scope, ownerCtx, kb, err := s.scanScope(ctx, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	scan, err := s.repo.CompleteScan(ownerCtx, scope, strings.TrimSpace(scanID))
	if err != nil {
		return nil, err
	}
	return s.scanView(ownerCtx, scope, kb, scan)
}

func (s *Service) scanScope(ctx context.Context, knowledgeBaseID string) (interfaces.LearningScope, context.Context, *types.KnowledgeBase, error) {
	callerScope, err := ResolveScope(ctx, knowledgeBaseID)
	if err != nil {
		return interfaces.LearningScope{}, nil, nil, err
	}
	if s.kbService == nil {
		return interfaces.LearningScope{}, nil, nil, errors.New("learning: knowledge base service is not configured")
	}
	kb, err := s.kbService.GetKnowledgeBaseByIDOnly(ctx, knowledgeBaseID)
	if err != nil || kb == nil || kb.TenantID == 0 {
		return interfaces.LearningScope{}, nil, nil, ErrLearningConceptNotFound
	}
	ownerCtx := context.WithValue(ctx, types.TenantIDContextKey, kb.TenantID)
	return interfaces.LearningScope{TenantID: kb.TenantID, SubjectID: callerScope.SubjectID, KnowledgeBaseID: knowledgeBaseID}, ownerCtx, kb, nil
}

// lockLearningKB bounds lock storage and serializes shared identity/bank initialization in one process.
func (s *Service) lockLearningKB(tenantID uint64, kbID string) func() {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d/%s", tenantID, kbID)))
	lock := &s.scanLocks[int(digest[0])%len(s.scanLocks)]
	lock.Lock()
	return lock.Unlock
}

func (s *Service) reconcileConceptIdentities(ctx context.Context, scope interfaces.LearningScope, kb *types.KnowledgeBase) ([]*types.LearningConceptIdentity, error) {
	identities, err := s.repo.ListConceptIdentities(ctx, kb.TenantID, scope.KnowledgeBaseID)
	if err != nil {
		return nil, err
	}
	// Reconcile missing published pages even when exposure already materialized a subset.
	if s.wikiRepo != nil {
		pages, pageErr := s.wikiRepo.ListByType(ctx, scope.KnowledgeBaseID, types.WikiPageTypeConcept)
		if pageErr != nil {
			return nil, pageErr
		}
		publishedPages := make(map[string]bool, len(pages))
		for _, page := range pages {
			if page != nil && page.TenantID == kb.TenantID && page.KnowledgeBaseID == scope.KnowledgeBaseID && page.Status == types.WikiPageStatusPublished {
				publishedPages[page.ID] = true
			}
		}
		existingPages := make(map[string]bool, len(identities))
		for _, identity := range identities {
			if identity != nil && identity.CurrentWikiPageID != nil {
				existingPages[*identity.CurrentWikiPageID] = true
			}
		}
		sort.Slice(pages, func(i, j int) bool {
			if pages[i] == nil {
				return false
			}
			if pages[j] == nil {
				return true
			}
			return pages[i].ID < pages[j].ID
		})
		for _, page := range pages {
			if page == nil || page.TenantID != kb.TenantID || page.KnowledgeBaseID != scope.KnowledgeBaseID || page.Status != types.WikiPageStatusPublished || existingPages[page.ID] {
				continue
			}
			lookup := interfaces.LearningConceptLookup{CurrentWikiPageID: page.ID, Slug: page.Slug, Title: page.Title, Aliases: append([]string(nil), page.Aliases...)}
			resolution, resolveErr := s.repo.ResolveConceptIdentity(ctx, kb.TenantID, scope.KnowledgeBaseID, lookup)
			if resolveErr != nil {
				return nil, resolveErr
			}
			// Names/aliases do not justify stealing the identity of another live page.
			if old := resolution.Identity; old != nil && old.CurrentWikiPageID != nil && *old.CurrentWikiPageID != page.ID && publishedPages[*old.CurrentWikiPageID] {
				id := page.ID
				if createErr := s.repo.CreateConceptIdentity(ctx, &types.LearningConceptIdentity{
					TenantID: kb.TenantID, KnowledgeBaseID: scope.KnowledgeBaseID, CurrentWikiPageID: &id,
					Slug: page.Slug, Title: page.Title, Aliases: append(types.StringArray(nil), page.Aliases...),
					LearningEligible: true, Status: types.LearningConceptIdentityActive,
				}); createErr != nil {
					return nil, createErr
				}
				continue
			}
			_, _, ensureErr := s.EnsureConceptIdentity(ctx, scope.KnowledgeBaseID, interfaces.LearningConceptLookup{
				CurrentWikiPageID: page.ID, Slug: page.Slug, Title: page.Title, Aliases: append([]string(nil), page.Aliases...),
			})
			if ensureErr != nil {
				return nil, ensureErr
			}
		}
		// Reload: an identity may have been rebound by slug instead of newly created.
		identities, err = s.repo.ListConceptIdentities(ctx, kb.TenantID, scope.KnowledgeBaseID)
		if err != nil {
			return nil, err
		}
	}
	return identities, nil
}

func (s *Service) scanCandidates(ctx context.Context, scope interfaces.LearningScope, kb *types.KnowledgeBase) ([]scanCandidate, error) {
	identities, err := s.reconcileConceptIdentities(ctx, scope, kb)
	if err != nil {
		return nil, err
	}
	states, err := s.repo.ListConceptStates(ctx, scope)
	if err != nil {
		return nil, err
	}
	stateByKey := make(map[string]*types.UserConceptState, len(states))
	for _, state := range states {
		if state != nil {
			stateByKey[state.ConceptKey] = state
		}
	}
	candidates := make([]scanCandidate, 0, len(identities))
	maxExposure, maxDegree := 0.0, 0
	for _, identity := range identities {
		if identity == nil || identity.CurrentWikiPageID == nil || !identity.LearningEligible || identity.Status != types.LearningConceptIdentityActive {
			continue
		}
		source, sourceErr := s.resolveQuizSource(ctx, scope.KnowledgeBaseID, identity.ConceptKey)
		if sourceErr != nil {
			continue
		}
		degree := len(source.page.InLinks) + len(source.page.OutLinks)
		candidate := scanCandidate{identity: identity, source: source, state: stateByKey[identity.ConceptKey], degree: degree}
		if candidate.state != nil && candidate.state.ExposureWeight > maxExposure {
			maxExposure = candidate.state.ExposureWeight
		}
		if degree > maxDegree {
			maxDegree = degree
		}
		candidates = append(candidates, candidate)
	}
	for index := range candidates {
		candidate := &candidates[index]
		confidence, exposure := 0.0, 0.0
		if candidate.state != nil {
			confidence = candidate.state.MasteryConfidence
			exposure = candidate.state.ExposureWeight
		}
		if maxExposure > 0 {
			exposure /= maxExposure
		}
		importance := 0.0
		if maxDegree > 0 {
			importance = float64(candidate.degree) / float64(maxDegree)
		}
		candidate.priority = 0.50*(1-confidence) + 0.35*exposure + 0.15*importance
	}
	sort.Slice(candidates, func(i, j int) bool {
		left, right := candidates[i], candidates[j]
		leftSeen := left.state != nil && left.state.Status != "" && left.state.Status != types.LearningProfileStatusUnseen
		rightSeen := right.state != nil && right.state.Status != "" && right.state.Status != types.LearningProfileStatusUnseen
		if leftSeen != rightSeen {
			return leftSeen
		}
		if left.priority == right.priority {
			return left.identity.ConceptKey < right.identity.ConceptKey
		}
		return left.priority > right.priority
	})
	return candidates, nil
}

func (s *Service) scanItems(ctx context.Context, scope interfaces.LearningScope, knowledgeBaseID string, candidate scanCandidate) ([]*types.QuizItem, error) {
	items, err := s.repo.ListQuizItems(ctx, candidate.source.kb.TenantID, knowledgeBaseID, candidate.identity.ConceptKey, candidate.source.sourceHash)
	if err != nil {
		return nil, err
	}
	if len(items) < scanItemsPerConcept {
		_, generateErr := s.GenerateQuizItems(ctx, knowledgeBaseID, candidate.identity.ConceptKey)
		if generateErr != nil {
			return nil, generateErr
		}
		items, err = s.repo.ListQuizItems(ctx, candidate.source.kb.TenantID, knowledgeBaseID, candidate.identity.ConceptKey, candidate.source.sourceHash)
		if err != nil {
			return nil, err
		}
	}
	attempts, err := s.repo.ListQuizAttempts(ctx, scope)
	if err != nil {
		return nil, err
	}
	attempted := make(map[string]struct{}, len(attempts))
	for _, attempt := range attempts {
		if attempt != nil {
			attempted[attempt.QuizItemID] = struct{}{}
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		_, leftAttempted := attempted[items[i].ID]
		_, rightAttempted := attempted[items[j].ID]
		if leftAttempted != rightAttempted {
			return !leftAttempted
		}
		return items[i].ID < items[j].ID
	})
	return items, nil
}

func (s *Service) scanView(ctx context.Context, scope interfaces.LearningScope, kb *types.KnowledgeBase, scan *types.LearningScan) (*types.LearningScanView, error) {
	if scan == nil {
		return nil, nil
	}
	identities, err := s.repo.ListConceptIdentities(ctx, kb.TenantID, scope.KnowledgeBaseID)
	if err != nil {
		return nil, err
	}
	titles := make(map[string]string, len(identities))
	itemsByID := make(map[string]*types.QuizItem, len(scan.QuizItemIDs))
	for _, identity := range identities {
		if identity == nil {
			continue
		}
		titles[identity.ConceptKey] = identity.Title
	}
	bankItems, err := s.repo.ListQuizItemsByID(ctx, kb.TenantID, scope.KnowledgeBaseID, []string(scan.QuizItemIDs))
	if err != nil {
		return nil, err
	}
	for _, item := range bankItems {
		itemsByID[item.ID] = item
	}

	items := make([]types.LearningScanItem, 0, len(scan.QuizItemIDs))
	for _, id := range scan.QuizItemIDs {
		item := itemsByID[id]
		if item == nil || (scan.Status != types.LearningScanStatusCompleted && (!item.IsActive || item.IsStale)) {
			return nil, interfaces.ErrLearningScanUnavailable
		}
		items = append(items, types.LearningScanItem{QuizItemView: types.QuizItemView{ID: item.ID, ConceptKey: item.ConceptKey, WikiPageID: item.WikiPageID, Question: item.Question, Options: item.Options, SourceChunkIDs: item.SourceChunkIDs, SourceHash: item.SourceHash, PromptVersion: item.PromptVersion, Difficulty: item.Difficulty, CreatedAt: item.CreatedAt}, ConceptTitle: titles[item.ConceptKey]})
	}
	states, err := s.repo.ListConceptStates(ctx, scope)
	if err != nil {
		return nil, err
	}
	scanConcepts := make(map[string]struct{}, len(items))
	for _, item := range items {
		scanConcepts[item.ConceptKey] = struct{}{}
	}
	summary := types.LearningScanSummary{}
	for _, state := range states {
		if state == nil {
			continue
		}
		if _, ok := scanConcepts[state.ConceptKey]; !ok {
			continue
		}
		switch state.Status {
		case types.LearningProfileStatusVerifiedStrong:
			summary.VerifiedStrong++
		case types.LearningProfileStatusVerifiedWeak:
			summary.VerifiedWeak++
		default:
			summary.Uncertain++
		}
	}
	return &types.LearningScanView{ID: scan.ID, Status: scan.Status, CurrentIndex: scan.CurrentIndex, TotalItems: len(items), Items: items, Summary: summary, CreatedAt: scan.CreatedAt, CompletedAt: scan.CompletedAt}, nil
}
