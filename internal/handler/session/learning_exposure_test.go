package session

import (
	"context"
	"errors"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/Tencent/WeKnora/internal/types/interfaces"
	"github.com/stretchr/testify/require"
)

type exposureMessageService struct {
	interfaces.MessageService
	updateErr error
}

func (s *exposureMessageService) UpdateMessage(
	_ context.Context, message *types.Message,
) error {
	return s.updateErr
}

func (s *exposureMessageService) IndexMessageToKB(
	context.Context, string, string, string, string,
) {
}

type exposureLearningService struct {
	interfaces.LearningService
	recorded []*types.Message
}

func (s *exposureLearningService) RecordDisplayedReferences(
	_ context.Context, message *types.Message,
) error {
	s.recorded = append(s.recorded, message)
	return nil
}

func TestCompleteAssistantMessageRecordsExposureOnlyAfterPersistence(t *testing.T) {
	learning := &exposureLearningService{}
	h := &Handler{
		messageService:  &exposureMessageService{},
		learningService: learning,
	}
	message := &types.Message{ID: "message-1", SessionID: "session-1", Role: "assistant"}
	h.completeAssistantMessage(context.Background(), message, "", "")
	require.True(t, message.IsCompleted)
	require.Len(t, learning.recorded, 1)
	require.Same(t, message, learning.recorded[0])
}

func TestCompleteAssistantMessageSkipsExposureWhenPersistenceFails(t *testing.T) {
	learning := &exposureLearningService{}
	h := &Handler{
		messageService:  &exposureMessageService{updateErr: errors.New("database unavailable")},
		learningService: learning,
	}
	h.completeAssistantMessage(context.Background(), &types.Message{
		ID: "message-1", SessionID: "session-1", Role: "assistant",
	}, "", "")
	require.Empty(t, learning.recorded)
}
