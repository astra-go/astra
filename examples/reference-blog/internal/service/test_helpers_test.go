package service_test

import (
	"context"
	"sync"
	"time"

	"github.com/astra-go/astra/cache"
	"github.com/astra-go/astra/mq"
	"github.com/astra-go/astra/search/elastic"
)

// mockProducer implements mq.Producer for testing
type mockProducer struct {
	mu       sync.Mutex
	messages []*mq.Message
}

func (m *mockProducer) Publish(_ context.Context, msg *mq.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msg)
	return nil
}

func (m *mockProducer) PublishBatch(_ context.Context, msgs []*mq.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, msgs...)
	return nil
}

func (m *mockProducer) Close() error { return nil }

// mockSearcher implements elastic.Searcher for testing
type mockSearcher struct{}

func (m *mockSearcher) Index(_ context.Context, _ elastic.IndexRequest) error { return nil }
func (m *mockSearcher) BulkIndex(_ context.Context, _ []elastic.IndexRequest) error {
	return nil
}
func (m *mockSearcher) Search(_ context.Context, _ elastic.SearchRequest) (*elastic.SearchResult, error) {
	return &elastic.SearchResult{}, nil
}
func (m *mockSearcher) Delete(_ context.Context, _, _ string) error    { return nil }
func (m *mockSearcher) DeleteIndex(_ context.Context, _ string) error  { return nil }
func (m *mockSearcher) CreateIndex(_ context.Context, _ string, _ map[string]any) error {
	return nil
}
func (m *mockSearcher) Close() error { return nil }

// MockCache implements cache.Cache for testing
type MockCache struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func NewMockCache() *MockCache {
	return &MockCache{data: make(map[string][]byte)}
}

func (m *MockCache) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		return nil, cache.ErrCacheMiss
	}
	return v, nil
}

func (m *MockCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func (m *MockCache) Delete(_ context.Context, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range keys {
		delete(m.data, k)
	}
	return nil
}

func (m *MockCache) Exists(_ context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.data[key]
	return ok, nil
}

func (m *MockCache) Flush(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = make(map[string][]byte)
	return nil
}

func (m *MockCache) Close() error { return nil }
