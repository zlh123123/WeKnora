package learning

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
)

var (
	ErrLearningTrackingDisabled = interfaces.ErrLearningTrackingDisabled
	ErrQuizItemUnavailable      = interfaces.ErrQuizItemUnavailable
	ErrLearningScanUnavailable  = interfaces.ErrLearningScanUnavailable
	ErrQuizItemNotInScan        = interfaces.ErrQuizItemNotInScan
	ErrInvalidQuizAttempt       = interfaces.ErrInvalidQuizAttempt
)

// SubmitQuizAttempt binds the client idempotency key to the authenticated
// caller and route resources before handing the atomic mutation to the repo.
func (s *Service) SubmitQuizAttempt(
	ctx context.Context, knowledgeBaseID, itemID string, request types.QuizAttemptRequest,
) (*types.QuizAttemptResult, error) {
	callerScope, err := ResolveScope(ctx, knowledgeBaseID)
	if err != nil {
		return nil, err
	}
	itemID = strings.TrimSpace(itemID)
	request.ScanID = strings.TrimSpace(request.ScanID)
	request.IdempotencyKey = strings.TrimSpace(request.IdempotencyKey)
	if itemID == "" || request.ScanID == "" || request.IdempotencyKey == "" ||
		request.SelectedOption < 0 || request.SelectedOption > 3 {
		return nil, ErrInvalidQuizAttempt
	}
	if s.kbService == nil {
		return nil, errors.New("learning: knowledge base service is not configured")
	}
	kb, err := s.kbService.GetKnowledgeBaseByIDOnly(ctx, knowledgeBaseID)
	if err != nil || kb == nil || kb.TenantID == 0 {
		return nil, ErrQuizItemUnavailable
	}
	ownerCtx := context.WithValue(ctx, types.TenantIDContextKey, kb.TenantID)
	scope := interfaces.LearningScope{
		TenantID: kb.TenantID, SubjectID: callerScope.SubjectID, KnowledgeBaseID: knowledgeBaseID,
	}
	digest := sha256.Sum256([]byte(fmt.Sprintf(
		"quiz-attempt-v1\x00%d\x00%s\x00%s\x00%s\x00%s\x00%s",
		scope.TenantID, scope.SubjectID, scope.KnowledgeBaseID, request.ScanID, itemID, request.IdempotencyKey,
	)))
	request.IdempotencyKey = fmt.Sprintf("%x", digest[:])
	return s.repo.SubmitQuizAttempt(ownerCtx, scope, itemID, request)
}
