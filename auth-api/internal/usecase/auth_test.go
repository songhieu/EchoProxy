package usecase

import (
	"context"
	"testing"
	"time"

	"github.com/songhieu/EchoProxy/auth-api/internal/domain"
)

type fakeUserRepo struct {
	store map[string]*domain.User
	id    uint64
}

func newFakeUserRepo() *fakeUserRepo { return &fakeUserRepo{store: map[string]*domain.User{}} }

func (f *fakeUserRepo) Create(_ context.Context, email, hash string) (*domain.User, error) {
	if _, ok := f.store[email]; ok {
		return nil, domain.ErrEmailTaken
	}
	f.id++
	u := &domain.User{ID: f.id, Email: email, PasswordHash: hash, CreatedAt: time.Now()}
	f.store[email] = u
	return u, nil
}
func (f *fakeUserRepo) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	u, ok := f.store[email]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}
func (f *fakeUserRepo) FindByID(_ context.Context, id uint64) (*domain.User, error) {
	for _, u := range f.store {
		if u.ID == id {
			return u, nil
		}
	}
	return nil, domain.ErrUserNotFound
}

func TestAuth_SignupAndLogin(t *testing.T) {
	a := NewAuth(newFakeUserRepo(), "0123456789abcdef0123456789abcdef", time.Hour)
	ctx := context.Background()

	u, tok, err := a.Signup(ctx, "alice@example.com", "password1234")
	if err != nil {
		t.Fatal(err)
	}
	if tok == "" {
		t.Fatal("empty token")
	}

	uid, err := a.Verify(tok)
	if err != nil {
		t.Fatal(err)
	}
	if uid != u.ID {
		t.Fatalf("user id mismatch: got %d want %d", uid, u.ID)
	}

	if _, _, err := a.Login(ctx, "alice@example.com", "password1234"); err != nil {
		t.Fatalf("login: %v", err)
	}
	if _, _, err := a.Login(ctx, "alice@example.com", "wrong"); err == nil {
		t.Fatalf("wrong password should fail")
	}
}

func TestGenerateAPIKey_Format(t *testing.T) {
	raw, hash, prefix, err := GenerateAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) < 32 || len(hash) != 64 || len(prefix) != 12 {
		t.Fatalf("unexpected key shape: raw=%d hash=%d prefix=%d", len(raw), len(hash), len(prefix))
	}
}
