package bot

import (
	"context"
	"testing"
	"time"

	"github.com/dyng/nosdaily/service"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockClient struct {
	mock.Mock
}

func (m *MockClient) Repost(ctx context.Context, sk, id, author string) error {
	args := m.Called(ctx, sk, id, author)
	return args.Error(0)
}

type MockService struct {
	mock.Mock
}

func (m *MockService) GetFeed() any {
	args := m.Called()
	return args.Get(0)
}

func TestWorkerRun(t *testing.T) {
	mockClient := new(MockClient)
	mockClient.On("Repost", context.Background(), "subSK", "123", "foo").Return(nil)

	mockService := new(MockService)
	mockService.On("GetFeed").Return([]service.FeedEntry{
		{
			Id:     "123",
			Pubkey: "foo",
		},
	})

	worker, err := NewWorker(context.Background(), mockClient, mockService)
	assert.NoError(t, err)

	worker.Run(context.Background(), "userPub", "subSK", time.Hour, 10)
	mockService.AssertCalled(t, "GetFeed")
	mockClient.AssertCalled(t, "Repost", context.Background(), "subSK", "123", "foo")
}
