package usecase

import (
	"context"
	"testing"

	"github.com/songhieu/EchoProxy/ingest-api/internal/domain"
	"github.com/songhieu/EchoProxy/pkg/event"
)

type fakeRepo struct{ m map[string]*domain.APIKey }

func (f *fakeRepo) GetByHash(_ context.Context, hash string) (*domain.APIKey, error) {
	k, ok := f.m[hash]
	if !ok {
		return nil, domain.ErrAPIKeyNotFound
	}
	return k, nil
}

type memCache struct{ m map[string]*domain.APIKey }

func newMemCache() *memCache                                       { return &memCache{m: map[string]*domain.APIKey{}} }
func (c *memCache) Get(h string) (*domain.APIKey, bool)            { v, ok := c.m[h]; return v, ok }
func (c *memCache) Set(h string, k *domain.APIKey)                 { c.m[h] = k }

type memSink struct{ events []*event.HttpEvent }

func (s *memSink) Push(_ context.Context, ev *event.HttpEvent) error {
	s.events = append(s.events, ev)
	return nil
}

func TestIngest_StampsIdentity(t *testing.T) {
	hash := HashKey("sk_test")
	repo := &fakeRepo{m: map[string]*domain.APIKey{
		hash: {ID: 7, ProjectID: 42, Hash: hash, Status: "active"},
	}}
	sink := &memSink{}
	uc := NewIngest(repo, newMemCache(), sink, nil, nil)
	res, err := uc.Execute(context.Background(), "sk_test", []*event.HttpEvent{
		{Method: "GET", Host: "api.example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Accepted != 1 {
		t.Fatalf("accepted=%d", res.Accepted)
	}
	if sink.events[0].ProjectId != 42 || sink.events[0].ApiKeyId != 7 {
		t.Fatalf("identity not stamped: %+v", sink.events[0])
	}
	if sink.events[0].EventId == "" {
		t.Fatal("event_id should be auto-filled")
	}
}

func TestIngest_RejectsRevoked(t *testing.T) {
	hash := HashKey("sk_test")
	repo := &fakeRepo{m: map[string]*domain.APIKey{
		hash: {ID: 1, Hash: hash, Status: "revoked"},
	}}
	uc := NewIngest(repo, newMemCache(), &memSink{}, nil, nil)
	if _, err := uc.Execute(context.Background(), "sk_test", []*event.HttpEvent{{Method: "GET", Host: "x"}}); err == nil {
		t.Fatal("expected error for revoked key")
	}
}

func TestIngest_BadEventCounts(t *testing.T) {
	hash := HashKey("sk_test")
	repo := &fakeRepo{m: map[string]*domain.APIKey{
		hash: {ID: 1, Hash: hash, Status: "active"},
	}}
	uc := NewIngest(repo, newMemCache(), &memSink{}, nil, nil)
	res, err := uc.Execute(context.Background(), "sk_test", []*event.HttpEvent{
		{Method: "", Host: ""}, // invalid
	})
	if err == nil {
		t.Fatal("expected error when all events rejected")
	}
	if res.Rejected != 1 {
		t.Fatalf("rejected=%d", res.Rejected)
	}
}
