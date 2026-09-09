package handler

import (
	stderrors "errors"
	"net/http"
	"strings"

	learningservice "github.com/Tencent/WeKnora/internal/application/service/learning"
	apperrors "github.com/Tencent/WeKnora/internal/errors"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/gin-gonic/gin"
)

// LearningHandler exposes caller-scoped Knowledge MRI reads. There is no
// subject parameter: ownership is always derived from the authenticated
// principal by the service layer.
type LearningHandler struct {
	service interfaces.LearningService
}

type SetLearningTrackingRequest struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

type SubmitQuizAttemptRequest struct {
	ScanID         string `json:"scan_id" binding:"required"`
	SelectedOption *int   `json:"selected_option" binding:"required"`
	IdempotencyKey string `json:"idempotency_key" binding:"required"`
}

func NewLearningHandler(service interfaces.LearningService) *LearningHandler {
	return &LearningHandler{service: service}
}

func (h *LearningHandler) GetProfile(c *gin.Context) {
	kbID, ok := learningKBID(c)
	if !ok {
		return
	}
	profile, err := h.service.GetProfile(c.Request.Context(), kbID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": profile})
}

func (h *LearningHandler) SetTracking(c *gin.Context) {
	kbID, ok := learningKBID(c)
	if !ok {
		return
	}
	var request SetLearningTrackingRequest
	if err := c.ShouldBindJSON(&request); err != nil || request.Enabled == nil {
		_ = c.Error(apperrors.NewBadRequestError("enabled is required"))
		return
	}
	profile, err := h.service.SetTracking(c.Request.Context(), kbID, *request.Enabled)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": profile})
}

func (h *LearningHandler) Clear(c *gin.Context) {
	kbID, ok := learningKBID(c)
	if !ok {
		return
	}
	profile, err := h.service.Clear(c.Request.Context(), kbID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": profile})
}

func (h *LearningHandler) Export(c *gin.Context) {
	kbID, ok := learningKBID(c)
	if !ok {
		return
	}
	export, err := h.service.Export(c.Request.Context(), kbID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", `attachment; filename="knowledge-mri-export.json"`)
	c.JSON(http.StatusOK, export)
}

// ListConceptStates returns the current Web user's materialized concept states
// for one knowledge base.
func (h *LearningHandler) ListConceptStates(c *gin.Context) {
	kbID, ok := learningKBID(c)
	if !ok {
		return
	}
	states, err := h.service.ListConceptStates(c.Request.Context(), kbID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	if states == nil {
		states = make([]*types.UserConceptState, 0)
	}
	c.JSON(http.StatusOK, gin.H{"data": states})
}

func (h *LearningHandler) GetLearningOverlay(c *gin.Context) {
	kbID, ok := learningKBID(c)
	if !ok {
		return
	}
	overlay, err := h.service.GetLearningOverlay(c.Request.Context(), kbID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": overlay})
}

func (h *LearningHandler) GetConceptInsights(c *gin.Context) {
	kbID, ok := learningKBID(c)
	if !ok {
		return
	}
	insights, err := h.service.GetConceptInsights(c.Request.Context(), kbID, strings.TrimSpace(c.Param("concept_key")))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": insights})
}

func (h *LearningHandler) GetQuizItems(c *gin.Context) {
	kbID, ok := learningKBID(c)
	if !ok {
		return
	}
	result, err := h.service.GetQuizItems(c.Request.Context(), kbID, strings.TrimSpace(c.Param("concept_key")))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *LearningHandler) GenerateQuizItems(c *gin.Context) {
	kbID, ok := learningKBID(c)
	if !ok {
		return
	}
	result, err := h.service.GenerateQuizItems(c.Request.Context(), kbID, strings.TrimSpace(c.Param("concept_key")))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *LearningHandler) SubmitQuizAttempt(c *gin.Context) {
	kbID, ok := learningKBID(c)
	if !ok {
		return
	}
	itemID := strings.TrimSpace(c.Param("item_id"))
	var request SubmitQuizAttemptRequest
	if itemID == "" || c.ShouldBindJSON(&request) != nil || request.SelectedOption == nil ||
		strings.TrimSpace(request.ScanID) == "" || strings.TrimSpace(request.IdempotencyKey) == "" {
		_ = c.Error(apperrors.NewBadRequestError("scan_id, selected_option and idempotency_key are required"))
		return
	}
	result, err := h.service.SubmitQuizAttempt(c.Request.Context(), kbID, itemID, types.QuizAttemptRequest{
		ScanID: request.ScanID, SelectedOption: *request.SelectedOption, IdempotencyKey: request.IdempotencyKey,
	})
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (h *LearningHandler) StartOrResumeScan(c *gin.Context) {
	kbID, ok := learningKBID(c)
	if !ok {
		return
	}
	scan, err := h.service.StartOrResumeScan(c.Request.Context(), kbID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": scan})
}

func (h *LearningHandler) GetActiveScan(c *gin.Context) {
	kbID, ok := learningKBID(c)
	if !ok {
		return
	}
	scan, err := h.service.GetActiveScan(c.Request.Context(), kbID)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": scan})
}

func (h *LearningHandler) CompleteScan(c *gin.Context) {
	kbID, ok := learningKBID(c)
	if !ok {
		return
	}
	scan, err := h.service.CompleteScan(c.Request.Context(), kbID, strings.TrimSpace(c.Param("scan_id")))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": scan})
}

func learningKBID(c *gin.Context) (string, bool) {
	kbID := strings.TrimSpace(c.Param("kb_id"))
	if kbID == "" {
		_ = c.Error(apperrors.NewBadRequestError("Knowledge base ID is required"))
		return "", false
	}
	return kbID, true
}

func (h *LearningHandler) writeError(c *gin.Context, err error) {
	switch {
	case stderrors.Is(err, learningservice.ErrUnsupportedPrincipal):
		_ = c.Error(apperrors.NewForbiddenError("Knowledge MRI is available only to signed-in Web users"))
	case stderrors.Is(err, learningservice.ErrNoLearningScope):
		_ = c.Error(apperrors.NewUnauthorizedError("Unauthorized"))
	case stderrors.Is(err, learningservice.ErrLearningConceptNotFound):
		_ = c.Error(apperrors.NewNotFoundError("Learning concept not found"))
	case stderrors.Is(err, learningservice.ErrQuizItemUnavailable):
		_ = c.Error(apperrors.NewNotFoundError("Quiz item not found or no longer active"))
	case stderrors.Is(err, learningservice.ErrLearningScanUnavailable):
		_ = c.Error(apperrors.NewNotFoundError("Learning scan not found or no longer active"))
	case stderrors.Is(err, learningservice.ErrInvalidQuizAttempt):
		_ = c.Error(apperrors.NewBadRequestError("Invalid quiz attempt"))
	case stderrors.Is(err, learningservice.ErrLearningTrackingDisabled):
		_ = c.Error(apperrors.NewConflictError("Learning tracking must be enabled before submitting an answer"))
	case stderrors.Is(err, learningservice.ErrQuizItemNotInScan):
		_ = c.Error(apperrors.NewConflictError("Quiz item does not belong to this scan"))
	case stderrors.Is(err, learningservice.ErrQuizCannotGenerate),
		stderrors.Is(err, learningservice.ErrInvalidQuizOutput):
		_ = c.Error(apperrors.NewConflictError(err.Error()))
	default:
		_ = c.Error(apperrors.NewInternalServerError(err.Error()))
	}
}
