package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type learningHandlerService struct {
	interfaces.LearningService
	states  []*types.UserConceptState
	kbID    string
	enabled *bool
	itemID  string
	attempt types.QuizAttemptRequest
	overlay *types.LearningOverlay
}

func (s *learningHandlerService) GetLearningOverlay(
	_ context.Context, kbID string,
) (*types.LearningOverlay, error) {
	s.kbID = kbID
	return s.overlay, nil
}

func (s *learningHandlerService) SubmitQuizAttempt(
	_ context.Context, kbID, itemID string, request types.QuizAttemptRequest,
) (*types.QuizAttemptResult, error) {
	s.kbID, s.itemID, s.attempt = kbID, itemID, request
	return &types.QuizAttemptResult{AttemptID: "attempt-1", IsCorrect: true}, nil
}

func (s *learningHandlerService) SetTracking(
	_ context.Context, kbID string, enabled bool,
) (*types.LearningProfile, error) {
	s.kbID = kbID
	s.enabled = &enabled
	return &types.LearningProfile{KnowledgeBaseID: kbID, TrackingEnabled: enabled}, nil
}

func (s *learningHandlerService) ListConceptStates(
	_ context.Context, kbID string,
) ([]*types.UserConceptState, error) {
	s.kbID = kbID
	return s.states, nil
}

func TestLearningHandlerAcceptsExplicitFalseTrackingValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &learningHandlerService{}
	h := NewLearningHandler(service)
	router := gin.New()
	router.PUT("/knowledgebase/:kb_id/learning/tracking", h.SetTracking)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPut,
		"/knowledgebase/kb-a/learning/tracking", strings.NewReader(`{"enabled":false}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.NotNil(t, service.enabled)
	require.False(t, *service.enabled)
}

func TestLearningHandlerListsCallerScopedStatesWithoutSubjectInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &learningHandlerService{states: []*types.UserConceptState{{
		ConceptKey: "concept-1", Status: types.LearningProfileStatusExposed,
	}}}
	h := NewLearningHandler(service)
	router := gin.New()
	router.GET("/knowledgebase/:kb_id/learning/concept-states", h.ListConceptStates)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet,
		"/knowledgebase/kb-a/learning/concept-states?subject_id=web_user:b", nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "kb-a", service.kbID)

	var response struct {
		Data []*types.UserConceptState `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.Len(t, response.Data, 1)
	require.Equal(t, "concept-1", response.Data[0].ConceptKey)
}

func TestLearningHandlerSubmitsQuizAttemptWithoutCallerScopeFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &learningHandlerService{}
	h := NewLearningHandler(service)
	router := gin.New()
	router.POST("/knowledgebase/:kb_id/learning/quiz/:item_id/attempt", h.SubmitQuizAttempt)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost,
		"/knowledgebase/kb-a/learning/quiz/item-1/attempt",
		strings.NewReader(`{"scan_id":"scan-1","selected_option":0,"idempotency_key":"request-1","subject_id":"web_user:bob","tenant_id":99}`))
	request.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "kb-a", service.kbID)
	require.Equal(t, "item-1", service.itemID)
	require.Equal(t, "scan-1", service.attempt.ScanID)
	require.Zero(t, service.attempt.SelectedOption)
	require.Equal(t, "request-1", service.attempt.IdempotencyKey)
}

func TestLearningHandlerReturnsBatchOverlay(t *testing.T) {
	gin.SetMode(gin.TestMode)
	service := &learningHandlerService{overlay: &types.LearningOverlay{
		TrackingEnabled: true,
		Items:           []types.LearningOverlayItem{{ConceptKey: "concept-1", WikiPageID: "page-1", Status: types.LearningProfileStatusExposed}},
	}}
	h := NewLearningHandler(service)
	router := gin.New()
	router.GET("/knowledgebase/:kb_id/wiki/learning-overlay", h.GetLearningOverlay)

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/knowledgebase/kb-a/wiki/learning-overlay?subject_id=web_user:bob", nil)
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	require.Equal(t, "kb-a", service.kbID)
	var response struct {
		Data types.LearningOverlay `json:"data"`
	}
	require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &response))
	require.True(t, response.Data.TrackingEnabled)
	require.Len(t, response.Data.Items, 1)
}
