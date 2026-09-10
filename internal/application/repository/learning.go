package repository

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type learningRepository struct {
	db           *gorm.DB
	attemptLocks sync.Map
}

var (
	ErrInvalidLearningScope    = errors.New("repository: invalid learning scope")
	ErrInvalidLearningConcept  = errors.New("repository: invalid learning concept")
	ErrInvalidLearningEvidence = errors.New("repository: invalid learning evidence")
)

func NewLearningRepository(db *gorm.DB) interfaces.LearningRepository {
	return &learningRepository{db: db}
}

func (r *learningRepository) attemptLock(scope interfaces.LearningScope) *sync.Mutex {
	key := fmt.Sprintf("%d:%s:%s", scope.TenantID, scope.SubjectID, scope.KnowledgeBaseID)
	lock, _ := r.attemptLocks.LoadOrStore(key, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func learningScoped(db *gorm.DB, ctx context.Context, scope interfaces.LearningScope) *gorm.DB {
	return db.WithContext(ctx).Where(
		"tenant_id = ? AND subject_id = ? AND knowledge_base_id = ?",
		scope.TenantID, scope.SubjectID, scope.KnowledgeBaseID,
	)
}

func (r *learningRepository) GetProfile(
	ctx context.Context, scope interfaces.LearningScope,
) (*types.LearningProfile, error) {
	if !scope.Valid() {
		return nil, ErrInvalidLearningScope
	}
	var profile types.LearningProfile
	err := learningScoped(r.db, ctx, scope).First(&profile).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func ensureLearningProfile(
	db *gorm.DB, ctx context.Context, scope interfaces.LearningScope,
) (*types.LearningProfile, error) {
	profile := &types.LearningProfile{
		ID:              uuid.NewString(),
		TenantID:        scope.TenantID,
		SubjectID:       scope.SubjectID,
		KnowledgeBaseID: scope.KnowledgeBaseID,
		TrackingEnabled: false,
	}
	if err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "tenant_id"}, {Name: "subject_id"}, {Name: "knowledge_base_id"},
		},
		DoNothing: true,
	}).Create(profile).Error; err != nil {
		return nil, err
	}
	var existing types.LearningProfile
	if err := learningScoped(db, ctx, scope).First(&existing).Error; err != nil {
		return nil, err
	}
	return &existing, nil
}

func (r *learningRepository) EnsureProfile(
	ctx context.Context, scope interfaces.LearningScope,
) (*types.LearningProfile, error) {
	if !scope.Valid() {
		return nil, ErrInvalidLearningScope
	}
	return ensureLearningProfile(r.db, ctx, scope)
}

func (r *learningRepository) SetTracking(
	ctx context.Context, scope interfaces.LearningScope, enabled bool,
) (*types.LearningProfile, error) {
	if !scope.Valid() {
		return nil, ErrInvalidLearningScope
	}
	var updated types.LearningProfile
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := ensureLearningProfile(tx, ctx, scope); err != nil {
			return err
		}
		var profile types.LearningProfile
		if err := learningScoped(tx, ctx, scope).
			Clauses(forUpdateClause()).
			First(&profile).Error; err != nil {
			return err
		}
		changes := map[string]any{
			"tracking_enabled": enabled,
			"updated_at":       time.Now(),
		}
		if enabled && !profile.TrackingEnabled {
			changes["enabled_at"] = time.Now()
		}
		if err := learningScoped(tx, ctx, scope).
			Model(&types.LearningProfile{}).
			Updates(changes).Error; err != nil {
			return err
		}
		return learningScoped(tx, ctx, scope).First(&updated).Error
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func (r *learningRepository) ClearScope(
	ctx context.Context, scope interfaces.LearningScope,
) (*types.LearningProfile, error) {
	if !scope.Valid() {
		return nil, ErrInvalidLearningScope
	}
	var updated types.LearningProfile
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := ensureLearningProfile(tx, ctx, scope); err != nil {
			return err
		}
		var profile types.LearningProfile
		if err := learningScoped(tx, ctx, scope).
			Clauses(forUpdateClause()).
			First(&profile).Error; err != nil {
			return err
		}
		for _, model := range []any{
			&types.LearningEvidence{},
			&types.QuizAttempt{},
			&types.LearningScan{},
			&types.UserConceptState{},
		} {
			if err := learningScoped(tx, ctx, scope).Delete(model).Error; err != nil {
				return err
			}
		}
		if err := learningScoped(tx, ctx, scope).
			Model(&types.LearningProfile{}).
			Updates(map[string]any{"cleared_at": time.Now(), "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return learningScoped(tx, ctx, scope).First(&updated).Error
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func NormalizeLearningSlug(value string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(value), "/"))
}

func NormalizeLearningName(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func prepareLearningIdentity(identity *types.LearningConceptIdentity) {
	if identity.ConceptKey == "" {
		identity.ConceptKey = uuid.NewString()
	}
	identity.Slug = strings.TrimSpace(identity.Slug)
	identity.NormalizedSlug = NormalizeLearningSlug(identity.Slug)
	identity.Title = strings.TrimSpace(identity.Title)
	identity.NormalizedTitle = NormalizeLearningName(identity.Title)
	if identity.Status == "" {
		identity.Status = types.LearningConceptIdentityActive
	}
	if identity.Aliases == nil {
		identity.Aliases = types.StringArray{}
	}
}

func (r *learningRepository) CreateConceptIdentity(
	ctx context.Context, identity *types.LearningConceptIdentity,
) error {
	prepareLearningIdentity(identity)
	if identity.TenantID == 0 || strings.TrimSpace(identity.KnowledgeBaseID) == "" ||
		(identity.CurrentWikiPageID == nil && identity.NormalizedSlug == "" && identity.NormalizedTitle == "") {
		return ErrInvalidLearningConcept
	}
	return r.db.WithContext(ctx).Create(identity).Error
}

func (r *learningRepository) UpdateConceptIdentity(
	ctx context.Context, identity *types.LearningConceptIdentity,
) error {
	prepareLearningIdentity(identity)
	if identity.TenantID == 0 || strings.TrimSpace(identity.KnowledgeBaseID) == "" || identity.ConceptKey == "" {
		return ErrInvalidLearningConcept
	}
	result := r.db.WithContext(ctx).
		Model(&types.LearningConceptIdentity{}).
		Where("tenant_id = ? AND knowledge_base_id = ? AND concept_key = ?",
			identity.TenantID, identity.KnowledgeBaseID, identity.ConceptKey).
		Updates(map[string]any{
			"current_wiki_page_id": identity.CurrentWikiPageID,
			"slug":                 identity.Slug,
			"normalized_slug":      identity.NormalizedSlug,
			"title":                identity.Title,
			"normalized_title":     identity.NormalizedTitle,
			"aliases":              identity.Aliases,
			"is_learning_eligible": identity.LearningEligible,
			"status":               identity.Status,
			"updated_at":           time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *learningRepository) GetConceptIdentity(
	ctx context.Context, tenantID uint64, knowledgeBaseID, conceptKey string,
) (*types.LearningConceptIdentity, error) {
	if tenantID == 0 || strings.TrimSpace(knowledgeBaseID) == "" || strings.TrimSpace(conceptKey) == "" {
		return nil, ErrInvalidLearningConcept
	}
	var identity types.LearningConceptIdentity
	err := r.db.WithContext(ctx).Where(
		"tenant_id = ? AND knowledge_base_id = ? AND concept_key = ?",
		tenantID, knowledgeBaseID, conceptKey,
	).First(&identity).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &identity, nil
}

func (r *learningRepository) ListConceptIdentities(
	ctx context.Context, tenantID uint64, knowledgeBaseID string,
) ([]*types.LearningConceptIdentity, error) {
	if tenantID == 0 || strings.TrimSpace(knowledgeBaseID) == "" {
		return nil, ErrInvalidLearningConcept
	}
	var identities []*types.LearningConceptIdentity
	err := r.db.WithContext(ctx).
		Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, knowledgeBaseID).
		Order("slug ASC, concept_key ASC").Find(&identities).Error
	return identities, err
}

func (r *learningRepository) ListQuizItems(
	ctx context.Context, tenantID uint64, knowledgeBaseID, conceptKey, sourceHash string,
) ([]*types.QuizItem, error) {
	if tenantID == 0 || strings.TrimSpace(knowledgeBaseID) == "" || strings.TrimSpace(conceptKey) == "" {
		return nil, ErrInvalidLearningConcept
	}
	query := r.db.WithContext(ctx).Where(
		"tenant_id = ? AND knowledge_base_id = ? AND concept_key = ?",
		tenantID, knowledgeBaseID, conceptKey,
	)
	if sourceHash != "" {
		query = query.Where("source_hash = ? AND is_active = ? AND is_stale = ?", sourceHash, true, false)
	}
	var items []*types.QuizItem
	if err := query.Order("created_at ASC, id ASC").Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

// ListQuizItemsByID loads only the persisted scan's items, including completed history.
func (r *learningRepository) ListQuizItemsByID(ctx context.Context, tenantID uint64, knowledgeBaseID string, ids []string) ([]*types.QuizItem, error) {
	items := make([]*types.QuizItem, 0)
	if tenantID == 0 || knowledgeBaseID == "" {
		return nil, ErrInvalidLearningScope
	}
	if len(ids) == 0 {
		return items, nil
	}
	err := r.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_base_id = ? AND id IN ?", tenantID, knowledgeBaseID, ids).Find(&items).Error
	return items, err
}

func (r *learningRepository) SaveQuizItems(
	ctx context.Context, tenantID uint64, knowledgeBaseID, conceptKey, sourceHash string, items []*types.QuizItem,
) ([]*types.QuizItem, error) {
	if tenantID == 0 || strings.TrimSpace(knowledgeBaseID) == "" || strings.TrimSpace(conceptKey) == "" || sourceHash == "" {
		return nil, ErrInvalidLearningConcept
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&types.QuizItem{}).Where(
			"tenant_id = ? AND knowledge_base_id = ? AND concept_key = ? AND is_active = ? AND is_stale = ? AND source_hash <> ?",
			tenantID, knowledgeBaseID, conceptKey, true, false, sourceHash,
		).Updates(map[string]any{"is_stale": true, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		for _, item := range items {
			if item == nil {
				continue
			}
			item.TenantID = tenantID
			item.KnowledgeBaseID = knowledgeBaseID
			item.ConceptKey = conceptKey
			item.SourceHash = sourceHash
			if item.ID == "" {
				item.ID = uuid.NewString()
			}
			item.IsActive = true
			item.IsStale = false
			if err := tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "tenant_id"}, {Name: "knowledge_base_id"}, {Name: "concept_key"}, {Name: "question_hash"}},
				DoNothing: true,
			}).Create(item).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.ListQuizItems(ctx, tenantID, knowledgeBaseID, conceptKey, sourceHash)
}

func masterySnapshot(state *types.UserConceptState) types.MasteryStateSnapshot {
	if state == nil {
		return types.MasteryStateSnapshot{Status: types.LearningProfileStatusUnseen}
	}
	return types.MasteryStateSnapshot{
		Status: state.Status, VerifiedMastery: state.VerifiedMastery,
		MasteryConfidence: state.MasteryConfidence, QuizAttemptCount: state.QuizAttemptCount,
	}
}

func snapshotMetadata(prefix string, snapshot types.MasteryStateSnapshot) types.JSONMap {
	return types.JSONMap{
		prefix + "_status":             snapshot.Status,
		prefix + "_verified_mastery":   snapshot.VerifiedMastery,
		prefix + "_mastery_confidence": snapshot.MasteryConfidence,
		prefix + "_quiz_attempt_count": snapshot.QuizAttemptCount,
	}
}

func metadataFloat(metadata types.JSONMap, key string) float64 {
	switch value := metadata[key].(type) {
	case float64:
		return value
	case float32:
		return float64(value)
	case int:
		return float64(value)
	case int64:
		return float64(value)
	case uint64:
		return float64(value)
	default:
		return 0
	}
}

func snapshotFromMetadata(metadata types.JSONMap, prefix string) types.MasteryStateSnapshot {
	status, _ := metadata[prefix+"_status"].(string)
	return types.MasteryStateSnapshot{
		Status:            status,
		VerifiedMastery:   metadataFloat(metadata, prefix+"_verified_mastery"),
		MasteryConfidence: metadataFloat(metadata, prefix+"_mastery_confidence"),
		QuizAttemptCount:  int(metadataFloat(metadata, prefix+"_quiz_attempt_count")),
	}
}

func quizAttemptResultFromEvidence(
	attempt *types.QuizAttempt, evidence *types.LearningEvidence, explanation string, idempotent bool,
) *types.QuizAttemptResult {
	previous := snapshotFromMetadata(evidence.Metadata, "previous")
	next := snapshotFromMetadata(evidence.Metadata, "new")
	return &types.QuizAttemptResult{
		AttemptID: attempt.ID, IsCorrect: attempt.IsCorrect, Explanation: explanation,
		PreviousState: previous, NewState: next, VerifiedMastery: next.VerifiedMastery,
		MasteryConfidence: next.MasteryConfidence, Idempotent: idempotent,
	}
}

// SubmitQuizAttempt is the sole production writer for verified mastery. The
// profile lock serializes all answer projections for one caller+KB; the local
// lock provides the same guarantee for SQLite, where FOR UPDATE is a no-op.
func (r *learningRepository) SubmitQuizAttempt(
	ctx context.Context, scope interfaces.LearningScope, itemID string, request types.QuizAttemptRequest,
) (*types.QuizAttemptResult, error) {
	itemID = strings.TrimSpace(itemID)
	request.ScanID = strings.TrimSpace(request.ScanID)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if !scope.Valid() || itemID == "" || request.ScanID == "" || request.IdempotencyKey == "" ||
		request.SelectedOption < 0 || request.SelectedOption > 3 {
		return nil, interfaces.ErrInvalidQuizAttempt
	}

	lock := r.attemptLock(scope)
	lock.Lock()
	defer lock.Unlock()

	var result *types.QuizAttemptResult
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var profile types.LearningProfile
		err := learningScoped(tx, ctx, scope).Clauses(forUpdateClause()).First(&profile).Error
		if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && !profile.TrackingEnabled) {
			return interfaces.ErrLearningTrackingDisabled
		}
		if err != nil {
			return err
		}

		var existing types.QuizAttempt
		err = learningScoped(tx, ctx, scope).
			Where("idempotency_key = ?", request.IdempotencyKey).First(&existing).Error
		if err == nil {
			var evidence types.LearningEvidence
			if err := learningScoped(tx, ctx, scope).
				Where("quiz_attempt_id = ? AND event_type = ?", existing.ID, types.LearningEvidenceQuizAttempt).
				First(&evidence).Error; err != nil {
				return err
			}
			var item types.QuizItem
			if err := tx.Where("tenant_id = ? AND knowledge_base_id = ? AND id = ?",
				scope.TenantID, scope.KnowledgeBaseID, existing.QuizItemID).First(&item).Error; err != nil {
				return err
			}
			result = quizAttemptResultFromEvidence(&existing, &evidence, item.Explanation, true)
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var item types.QuizItem
		err = tx.Where(
			"tenant_id = ? AND knowledge_base_id = ? AND id = ? AND is_active = ? AND is_stale = ?",
			scope.TenantID, scope.KnowledgeBaseID, itemID, true, false,
		).First(&item).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return interfaces.ErrQuizItemUnavailable
		}
		if err != nil {
			return err
		}
		if request.SelectedOption >= len(item.Options) {
			return interfaces.ErrInvalidQuizAttempt
		}

		var scan types.LearningScan
		err = learningScoped(tx, ctx, scope).
			Where("id = ? AND status IN ?", request.ScanID,
				[]string{types.LearningScanStatusPending, types.LearningScanStatusActive}).
			First(&scan).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return interfaces.ErrLearningScanUnavailable
		}
		if err != nil {
			return err
		}
		inScan := false
		for _, scanItemID := range scan.QuizItemIDs {
			if scanItemID == item.ID {
				inScan = true
				break
			}
		}
		if !inScan {
			return interfaces.ErrQuizItemNotInScan
		}
		var priorAnswerCount int64
		if err := learningScoped(tx, ctx, scope).Model(&types.QuizAttempt{}).
			Where("scan_id = ? AND quiz_item_id = ?", scan.ID, item.ID).Count(&priorAnswerCount).Error; err != nil {
			return err
		}
		if priorAnswerCount > 0 {
			return interfaces.ErrInvalidQuizAttempt
		}

		var state types.UserConceptState
		err = learningScoped(tx, ctx, scope).Where("concept_key = ?", item.ConceptKey).First(&state).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			state = types.UserConceptState{
				ID: uuid.NewString(), TenantID: scope.TenantID, SubjectID: scope.SubjectID,
				KnowledgeBaseID: scope.KnowledgeBaseID, ConceptKey: item.ConceptKey,
				Status: types.LearningProfileStatusUnseen,
			}
		} else if err != nil {
			return err
		}
		previous := masterySnapshot(&state)
		now := time.Now()
		attempt := &types.QuizAttempt{
			ID: uuid.NewString(), TenantID: scope.TenantID, SubjectID: scope.SubjectID,
			KnowledgeBaseID: scope.KnowledgeBaseID, QuizItemID: item.ID, ScanID: scan.ID,
			ConceptKey: item.ConceptKey, SelectedOption: request.SelectedOption,
			IsCorrect: request.SelectedOption == item.CorrectOption, AnsweredAt: now,
			IdempotencyKey: request.IdempotencyKey,
		}
		if attempt.IsCorrect {
			attempt.Score = 1
		}
		if err := tx.Create(attempt).Error; err != nil {
			return err
		}

		var recent []types.QuizAttempt
		if err := tx.WithContext(ctx).
			Table("quiz_attempts AS a").Select("a.*").
			Joins("JOIN quiz_items AS q ON q.id = a.quiz_item_id").
			Where("a.tenant_id = ? AND a.subject_id = ? AND a.knowledge_base_id = ? AND a.concept_key = ? AND q.tenant_id = ? AND q.knowledge_base_id = ? AND q.source_hash = ? AND q.is_active = ? AND q.is_stale = ?",
				scope.TenantID, scope.SubjectID, scope.KnowledgeBaseID, item.ConceptKey,
				scope.TenantID, scope.KnowledgeBaseID, item.SourceHash, true, false).
			Order("a.answered_at DESC, a.created_at DESC, a.id DESC").Limit(4).Find(&recent).Error; err != nil {
			return err
		}
		correct := 0
		for i := range recent {
			if recent[i].IsCorrect {
				correct++
			}
		}
		mastery := float64(correct) / float64(len(recent))
		confidence := float64(len(recent)) / 4
		status := types.LearningProfileStatusUncertain
		if len(recent) >= 2 && mastery >= 0.8 {
			status = types.LearningProfileStatusVerifiedStrong
		} else if len(recent) >= 2 && mastery <= 0.4 {
			status = types.LearningProfileStatusVerifiedWeak
		}
		var totalAttempts int64
		if err := learningScoped(tx, ctx, scope).Model(&types.QuizAttempt{}).
			Where("concept_key = ?", item.ConceptKey).Count(&totalAttempts).Error; err != nil {
			return err
		}
		next := types.MasteryStateSnapshot{
			Status: status, VerifiedMastery: mastery, MasteryConfidence: confidence,
			QuizAttemptCount: int(totalAttempts),
		}
		state.VerifiedMastery = mastery
		state.MasteryConfidence = confidence
		state.QuizAttemptCount = int(totalAttempts)
		state.LastAssessedAt = &now
		state.Status = status
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "tenant_id"}, {Name: "subject_id"}, {Name: "knowledge_base_id"}, {Name: "concept_key"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"verified_mastery", "mastery_confidence", "quiz_attempt_count", "last_assessed_at", "status", "updated_at",
			}),
		}).Create(&state).Error; err != nil {
			return err
		}

		metadata := snapshotMetadata("previous", previous)
		for key, value := range snapshotMetadata("new", next) {
			metadata[key] = value
		}
		evidence := &types.LearningEvidence{
			ID: uuid.NewString(), TenantID: scope.TenantID, SubjectID: scope.SubjectID,
			KnowledgeBaseID: scope.KnowledgeBaseID, ConceptKey: item.ConceptKey,
			WikiPageID: item.WikiPageID, QuizItemID: &item.ID, QuizAttemptID: &attempt.ID,
			EventType: types.LearningEvidenceQuizAttempt, EvidenceValue: attempt.Score,
			Confidence: 1, Metadata: metadata, OccurredAt: now,
			IdempotencyKey: "evidence:" + request.IdempotencyKey,
		}
		if err := tx.Create(evidence).Error; err != nil {
			return err
		}
		var answered int64
		if err := learningScoped(tx, ctx, scope).Model(&types.QuizAttempt{}).
			Where("scan_id = ?", scan.ID).Count(&answered).Error; err != nil {
			return err
		}
		scanUpdates := map[string]any{"current_index": int(answered), "updated_at": now}
		if int(answered) >= len(scan.QuizItemIDs) {
			scanUpdates["status"] = types.LearningScanStatusCompleted
			scanUpdates["completed_at"] = now
		} else if scan.Status == types.LearningScanStatusPending {
			scanUpdates["status"] = types.LearningScanStatusActive
		}
		if err := learningScoped(tx, ctx, scope).Model(&types.LearningScan{}).
			Where("id = ?", scan.ID).Updates(scanUpdates).Error; err != nil {
			return err
		}
		result = quizAttemptResultFromEvidence(attempt, evidence, item.Explanation, false)
		return nil
	})
	return result, err
}

// GetActiveScan distinguishes the six-question scan from a two-question concept retest.
// The target is derived from persisted items, so existing scans need no migration.
func (r *learningRepository) GetActiveScan(ctx context.Context, scope interfaces.LearningScope, conceptKeys ...string) (*types.LearningScan, error) {
	if !scope.Valid() {
		return nil, ErrInvalidLearningScope
	}
	target := ""
	if len(conceptKeys) > 0 {
		target = strings.TrimSpace(conceptKeys[0])
	}
	var scans []*types.LearningScan
	if err := learningScoped(r.db, ctx, scope).Where("status IN ?", []string{types.LearningScanStatusPending, types.LearningScanStatusActive}).Order("created_at DESC, id DESC").Find(&scans).Error; err != nil {
		return nil, err
	}
	for _, scan := range scans {
		if target == "" {
			if len(scan.QuizItemIDs) == 6 {
				return scan, nil
			}
			continue
		}
		if len(scan.QuizItemIDs) != 2 {
			continue
		}
		var count int64
		if err := r.db.WithContext(ctx).Model(&types.QuizItem{}).Where("tenant_id = ? AND knowledge_base_id = ? AND concept_key = ? AND id IN ?", scope.TenantID, scope.KnowledgeBaseID, target, []string(scan.QuizItemIDs)).Count(&count).Error; err != nil {
			return nil, err
		}
		if count == 2 {
			return scan, nil
		}
	}
	return nil, nil
}

func (r *learningRepository) CreateScan(ctx context.Context, scope interfaces.LearningScope, scan *types.LearningScan) error {
	if !scope.Valid() || scan == nil || len(scan.QuizItemIDs) == 0 {
		return ErrInvalidLearningScope
	}
	scan.TenantID, scan.SubjectID, scan.KnowledgeBaseID = scope.TenantID, scope.SubjectID, scope.KnowledgeBaseID
	if scan.ID == "" {
		scan.ID = uuid.NewString()
	}
	if scan.Status == "" {
		scan.Status = types.LearningScanStatusPending
	}
	return r.db.WithContext(ctx).Create(scan).Error
}

func (r *learningRepository) GetScan(ctx context.Context, scope interfaces.LearningScope, scanID string) (*types.LearningScan, error) {
	if !scope.Valid() || strings.TrimSpace(scanID) == "" {
		return nil, ErrInvalidLearningScope
	}
	var scan types.LearningScan
	err := learningScoped(r.db, ctx, scope).Where("id = ?", scanID).First(&scan).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &scan, nil
}

func (r *learningRepository) ListQuizAttemptsForScan(ctx context.Context, scope interfaces.LearningScope, scanID string) ([]*types.QuizAttempt, error) {
	if !scope.Valid() || strings.TrimSpace(scanID) == "" {
		return nil, ErrInvalidLearningScope
	}
	var attempts []*types.QuizAttempt
	err := learningScoped(r.db, ctx, scope).Where("scan_id = ?", scanID).
		Order("answered_at ASC, created_at ASC, id ASC").Find(&attempts).Error
	return attempts, err
}

func (r *learningRepository) ListQuizAttempts(ctx context.Context, scope interfaces.LearningScope) ([]*types.QuizAttempt, error) {
	if !scope.Valid() {
		return nil, ErrInvalidLearningScope
	}
	var attempts []*types.QuizAttempt
	err := learningScoped(r.db, ctx, scope).Order("answered_at ASC, created_at ASC, id ASC").Find(&attempts).Error
	return attempts, err
}

func (r *learningRepository) CompleteScan(ctx context.Context, scope interfaces.LearningScope, scanID string) (*types.LearningScan, error) {
	if !scope.Valid() || strings.TrimSpace(scanID) == "" {
		return nil, ErrInvalidLearningScope
	}
	lock := r.attemptLock(scope)
	lock.Lock()
	defer lock.Unlock()
	var completed *types.LearningScan
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var scan types.LearningScan
		if err := learningScoped(tx, ctx, scope).Clauses(forUpdateClause()).Where("id = ?", scanID).First(&scan).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return interfaces.ErrLearningScanUnavailable
			}
			return err
		}
		var answered int64
		if err := learningScoped(tx, ctx, scope).Model(&types.QuizAttempt{}).Where("scan_id = ?", scan.ID).Count(&answered).Error; err != nil {
			return err
		}
		if int(answered) < len(scan.QuizItemIDs) {
			return interfaces.ErrLearningScanUnavailable
		}
		now := time.Now()
		if err := learningScoped(tx, ctx, scope).Model(&types.LearningScan{}).Where("id = ?", scan.ID).Updates(map[string]any{
			"status": types.LearningScanStatusCompleted, "current_index": int(answered), "completed_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		scan.Status, scan.CurrentIndex, scan.CompletedAt = types.LearningScanStatusCompleted, int(answered), &now
		completed = &scan
		return nil
	})
	return completed, err
}

func uniqueLearningResolution(
	identities []types.LearningConceptIdentity, matchedBy string,
) interfaces.LearningConceptResolution {
	if len(identities) == 1 {
		return interfaces.LearningConceptResolution{Identity: &identities[0], MatchedBy: matchedBy}
	}
	if len(identities) > 1 {
		return interfaces.LearningConceptResolution{
			MatchedBy: interfaces.LearningConceptMatchedAmbiguous,
			Ambiguous: true,
		}
	}
	return interfaces.LearningConceptResolution{}
}

func (r *learningRepository) ResolveConceptIdentity(
	ctx context.Context,
	tenantID uint64,
	knowledgeBaseID string,
	lookup interfaces.LearningConceptLookup,
) (interfaces.LearningConceptResolution, error) {
	if tenantID == 0 || strings.TrimSpace(knowledgeBaseID) == "" {
		return interfaces.LearningConceptResolution{}, ErrInvalidLearningConcept
	}
	scopeQuery := func() *gorm.DB {
		return r.db.WithContext(ctx).Where(
			"tenant_id = ? AND knowledge_base_id = ?", tenantID, knowledgeBaseID,
		)
	}
	if pageID := strings.TrimSpace(lookup.CurrentWikiPageID); pageID != "" {
		var identities []types.LearningConceptIdentity
		if err := scopeQuery().Where("current_wiki_page_id = ?", pageID).Find(&identities).Error; err != nil {
			return interfaces.LearningConceptResolution{}, err
		}
		if resolution := uniqueLearningResolution(identities, interfaces.LearningConceptMatchedPageID); resolution.Identity != nil || resolution.Ambiguous {
			return resolution, nil
		}
	}
	if slug := NormalizeLearningSlug(lookup.Slug); slug != "" {
		var identities []types.LearningConceptIdentity
		if err := scopeQuery().Where("normalized_slug = ?", slug).Find(&identities).Error; err != nil {
			return interfaces.LearningConceptResolution{}, err
		}
		if resolution := uniqueLearningResolution(identities, interfaces.LearningConceptMatchedSlug); resolution.Identity != nil || resolution.Ambiguous {
			return resolution, nil
		}
	}

	var identities []types.LearningConceptIdentity
	if err := scopeQuery().Find(&identities).Error; err != nil {
		return interfaces.LearningConceptResolution{}, err
	}
	lookupNames := make(map[string]struct{}, len(lookup.Aliases)+1)
	if title := NormalizeLearningName(lookup.Title); title != "" {
		lookupNames[title] = struct{}{}
	}
	for _, alias := range lookup.Aliases {
		if normalized := NormalizeLearningName(alias); normalized != "" {
			lookupNames[normalized] = struct{}{}
		}
	}
	aliasMatches := make([]types.LearningConceptIdentity, 0, 1)
	for _, identity := range identities {
		matched := false
		for _, alias := range identity.Aliases {
			if _, ok := lookupNames[NormalizeLearningName(alias)]; ok {
				matched = true
				break
			}
		}
		if matched {
			aliasMatches = append(aliasMatches, identity)
		}
	}
	if resolution := uniqueLearningResolution(aliasMatches, interfaces.LearningConceptMatchedAlias); resolution.Identity != nil || resolution.Ambiguous {
		return resolution, nil
	}

	titleMatches := make([]types.LearningConceptIdentity, 0, 1)
	for _, identity := range identities {
		if _, ok := lookupNames[identity.NormalizedTitle]; ok && identity.NormalizedTitle != "" {
			titleMatches = append(titleMatches, identity)
		}
	}
	if resolution := uniqueLearningResolution(titleMatches, interfaces.LearningConceptMatchedTitle); resolution.Identity != nil || resolution.Ambiguous {
		return resolution, nil
	}
	return interfaces.LearningConceptResolution{MatchedBy: interfaces.LearningConceptMatchedUnresolved}, nil
}

func (r *learningRepository) GetConceptState(
	ctx context.Context, scope interfaces.LearningScope, conceptKey string,
) (*types.UserConceptState, error) {
	if !scope.Valid() {
		return nil, ErrInvalidLearningScope
	}
	var state types.UserConceptState
	err := learningScoped(r.db, ctx, scope).Where("concept_key = ?", conceptKey).First(&state).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (r *learningRepository) ListConceptStates(
	ctx context.Context, scope interfaces.LearningScope,
) ([]*types.UserConceptState, error) {
	if !scope.Valid() {
		return nil, ErrInvalidLearningScope
	}
	var states []*types.UserConceptState
	err := learningScoped(r.db, ctx, scope).Order("concept_key ASC").Find(&states).Error
	return states, err
}

func (r *learningRepository) AppendEvidence(
	ctx context.Context, scope interfaces.LearningScope, evidence *types.LearningEvidence,
) (bool, error) {
	if !scope.Valid() {
		return false, ErrInvalidLearningScope
	}
	if strings.TrimSpace(evidence.ConceptKey) == "" || strings.TrimSpace(evidence.EventType) == "" ||
		strings.TrimSpace(evidence.IdempotencyKey) == "" {
		return false, ErrInvalidLearningEvidence
	}
	inserted := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var profile types.LearningProfile
		err := learningScoped(tx, ctx, scope).
			Clauses(forUpdateClause()).
			First(&profile).Error
		if errors.Is(err, gorm.ErrRecordNotFound) || !profile.TrackingEnabled {
			return nil
		}
		if err != nil {
			return err
		}
		if evidence.OccurredAt.IsZero() {
			evidence.OccurredAt = time.Now()
		}
		if profile.EnabledAt != nil && evidence.OccurredAt.Before(*profile.EnabledAt) {
			return nil
		}
		if profile.ClearedAt != nil && !evidence.OccurredAt.After(*profile.ClearedAt) {
			return nil
		}
		evidence.TenantID = scope.TenantID
		evidence.SubjectID = scope.SubjectID
		evidence.KnowledgeBaseID = scope.KnowledgeBaseID
		if evidence.ID == "" {
			evidence.ID = uuid.NewString()
		}
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "idempotency_key"}},
			DoNothing: true,
		}).Create(evidence)
		if result.Error != nil {
			return result.Error
		}
		inserted = result.RowsAffected == 1
		return nil
	})
	return inserted, err
}

// AppendExposureEvidence atomically appends one displayed-reference evidence
// row and updates only the exposure projection. The profile row serializes
// writes for a caller+KB scope, so retries cannot double-increment the state.
// ExposureCount counts distinct assistant messages per concept, while
// ExposureWeight accumulates each candidate chunk's confidence.
func (r *learningRepository) AppendExposureEvidence(
	ctx context.Context, scope interfaces.LearningScope, evidence *types.LearningEvidence,
) (bool, error) {
	if !scope.Valid() {
		return false, ErrInvalidLearningScope
	}
	if evidence == nil || evidence.EventType != types.LearningEvidenceDisplayedReference ||
		strings.TrimSpace(evidence.ConceptKey) == "" || strings.TrimSpace(evidence.IdempotencyKey) == "" ||
		evidence.MessageID == nil || strings.TrimSpace(*evidence.MessageID) == "" ||
		evidence.ChunkID == nil || strings.TrimSpace(*evidence.ChunkID) == "" ||
		evidence.Confidence <= 0 || evidence.Confidence > 1 {
		return false, ErrInvalidLearningEvidence
	}

	inserted := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var profile types.LearningProfile
		err := learningScoped(tx, ctx, scope).
			Clauses(forUpdateClause()).
			First(&profile).Error
		if errors.Is(err, gorm.ErrRecordNotFound) || !profile.TrackingEnabled {
			return nil
		}
		if err != nil {
			return err
		}
		if evidence.OccurredAt.IsZero() {
			evidence.OccurredAt = time.Now()
		}
		if profile.EnabledAt != nil && evidence.OccurredAt.Before(*profile.EnabledAt) {
			return nil
		}
		if profile.ClearedAt != nil && !evidence.OccurredAt.After(*profile.ClearedAt) {
			return nil
		}

		var priorMessageEvidence int64
		if err := learningScoped(tx, ctx, scope).
			Model(&types.LearningEvidence{}).
			Where("concept_key = ? AND event_type = ? AND message_id = ?",
				evidence.ConceptKey, types.LearningEvidenceDisplayedReference, *evidence.MessageID).
			Count(&priorMessageEvidence).Error; err != nil {
			return err
		}

		evidence.TenantID = scope.TenantID
		evidence.SubjectID = scope.SubjectID
		evidence.KnowledgeBaseID = scope.KnowledgeBaseID
		if evidence.ID == "" {
			evidence.ID = uuid.NewString()
		}
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "idempotency_key"}},
			DoNothing: true,
		}).Create(evidence)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		inserted = true

		var state types.UserConceptState
		err = learningScoped(tx, ctx, scope).
			Clauses(forUpdateClause()).
			Where("concept_key = ?", evidence.ConceptKey).
			First(&state).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			state = types.UserConceptState{
				ID:              uuid.NewString(),
				TenantID:        scope.TenantID,
				SubjectID:       scope.SubjectID,
				KnowledgeBaseID: scope.KnowledgeBaseID,
				ConceptKey:      evidence.ConceptKey,
				ExposureCount:   1,
				ExposureWeight:  evidence.Confidence,
				LastExposedAt:   &evidence.OccurredAt,
				Status:          types.LearningProfileStatusExposed,
			}
			return tx.Create(&state).Error
		}
		if err != nil {
			return err
		}

		count := state.ExposureCount
		if priorMessageEvidence == 0 {
			count++
		}
		lastExposedAt := state.LastExposedAt
		if lastExposedAt == nil || evidence.OccurredAt.After(*lastExposedAt) {
			lastExposedAt = &evidence.OccurredAt
		}
		status := state.Status
		if status == "" || status == types.LearningProfileStatusUnseen {
			status = types.LearningProfileStatusExposed
		}
		return learningScoped(tx, ctx, scope).
			Model(&types.UserConceptState{}).
			Where("concept_key = ?", evidence.ConceptKey).
			Updates(map[string]any{
				"exposure_count":  count,
				"exposure_weight": state.ExposureWeight + evidence.Confidence,
				"last_exposed_at": lastExposedAt,
				"status":          status,
				"updated_at":      time.Now(),
			}).Error
	})
	return inserted, err
}

func (r *learningRepository) CountEvidence(
	ctx context.Context, scope interfaces.LearningScope,
) (int64, error) {
	if !scope.Valid() {
		return 0, ErrInvalidLearningScope
	}
	var count int64
	err := learningScoped(r.db, ctx, scope).Model(&types.LearningEvidence{}).Count(&count).Error
	return count, err
}

func (r *learningRepository) ListEvidence(
	ctx context.Context, scope interfaces.LearningScope, conceptKey string, limit int,
) ([]*types.LearningEvidence, error) {
	if !scope.Valid() || strings.TrimSpace(conceptKey) == "" {
		return nil, ErrInvalidLearningScope
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var evidence []*types.LearningEvidence
	err := learningScoped(r.db, ctx, scope).
		Where("concept_key = ?", strings.TrimSpace(conceptKey)).
		Order("occurred_at DESC, created_at DESC, id DESC").
		Limit(limit).Find(&evidence).Error
	return evidence, err
}

func (r *learningRepository) ExportScope(ctx context.Context, scope interfaces.LearningScope) (*types.LearningExport, error) {
	if !scope.Valid() {
		return nil, ErrInvalidLearningScope
	}
	profile, err := r.GetProfile(ctx, scope)
	if err != nil {
		return nil, err
	}
	states, err := r.ListConceptStates(ctx, scope)
	if err != nil {
		return nil, err
	}
	evidence := make([]*types.LearningEvidence, 0)
	if err := learningScoped(r.db, ctx, scope).Order("occurred_at ASC, created_at ASC, id ASC").Find(&evidence).Error; err != nil {
		return nil, err
	}
	attempts, err := r.ListQuizAttempts(ctx, scope)
	if err != nil {
		return nil, err
	}
	items := make([]*types.QuizItem, 0)
	if err := r.db.WithContext(ctx).Where("tenant_id = ? AND knowledge_base_id = ?", scope.TenantID, scope.KnowledgeBaseID).Find(&items).Error; err != nil {
		return nil, err
	}
	views := make([]types.QuizItemView, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		views = append(views, types.QuizItemView{ID: item.ID, ConceptKey: item.ConceptKey, WikiPageID: item.WikiPageID, Question: item.Question, Options: item.Options, SourceChunkIDs: item.SourceChunkIDs, SourceHash: item.SourceHash, PromptVersion: item.PromptVersion, Difficulty: item.Difficulty, CreatedAt: item.CreatedAt})
	}
	return &types.LearningExport{ExportedAt: time.Now(), Profile: profile, ConceptStates: states, Evidence: evidence, QuizAttempts: attempts, QuizItems: views}, nil
}

func (r *learningRepository) DeleteByKnowledgeBase(ctx context.Context, tenantID uint64, knowledgeBaseID string) error {
	if tenantID == 0 || strings.TrimSpace(knowledgeBaseID) == "" {
		return ErrInvalidLearningScope
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, model := range []any{&types.LearningEvidence{}, &types.QuizAttempt{}, &types.LearningScan{}, &types.UserConceptState{}, &types.LearningProfile{}} {
			if err := tx.Where("tenant_id = ? AND knowledge_base_id = ?", tenantID, knowledgeBaseID).Delete(model).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *learningRepository) DeleteByTenant(ctx context.Context, tenantID uint64) error {
	if tenantID == 0 {
		return ErrInvalidLearningScope
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, model := range []any{&types.LearningEvidence{}, &types.QuizAttempt{}, &types.LearningScan{}, &types.UserConceptState{}, &types.LearningProfile{}, &types.LearningConceptIdentity{}, &types.QuizItem{}} {
			if err := tx.Where("tenant_id = ?", tenantID).Delete(model).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
