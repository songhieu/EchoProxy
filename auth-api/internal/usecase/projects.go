package usecase

import (
	"context"

	"github.com/songhieu/EchoProxy/auth-api/internal/domain"
)

type Projects struct{ repo domain.ProjectRepository }

func NewProjects(repo domain.ProjectRepository) *Projects { return &Projects{repo: repo} }

func (p *Projects) Create(ctx context.Context, ownerID uint64, name string) (*domain.Project, error) {
	if name == "" {
		name = "Default"
	}
	return p.repo.Create(ctx, ownerID, name)
}

func (p *Projects) List(ctx context.Context, ownerID uint64) ([]*domain.Project, error) {
	return p.repo.List(ctx, ownerID)
}

func (p *Projects) Get(ctx context.Context, id, ownerID uint64) (*domain.Project, error) {
	return p.repo.Get(ctx, id, ownerID)
}

func (p *Projects) UpdateRetention(ctx context.Context, id, ownerID uint64, days int) (*domain.Project, error) {
	return p.repo.UpdateRetention(ctx, id, ownerID, days)
}

// Delete removes the project + cascades its API keys (Postgres FK with
// ON DELETE CASCADE). ClickHouse events stay until retention expires —
// they're tied to project_id and the cleanup job drops them based on
// the project's retention window when the project is gone.
func (p *Projects) Delete(ctx context.Context, id, ownerID uint64) error {
	return p.repo.Delete(ctx, id, ownerID)
}

type APIKeys struct {
	keys     domain.APIKeyRepository
	projects domain.ProjectRepository
}

func NewAPIKeys(keys domain.APIKeyRepository, projects domain.ProjectRepository) *APIKeys {
	return &APIKeys{keys: keys, projects: projects}
}

type CreateAPIKeyInput struct {
	ProjectID    uint64
	OwnerID      uint64
	Allowlist    []string
	BodyCap      int
	RateLimitRPS int
	RedactRules  []byte
	Description  string
}

type CreateAPIKeyOutput struct {
	Key *domain.APIKey
	Raw string // shown to the user once
}

func (a *APIKeys) Create(ctx context.Context, in CreateAPIKeyInput) (*CreateAPIKeyOutput, error) {
	if _, err := a.projects.Get(ctx, in.ProjectID, in.OwnerID); err != nil {
		return nil, err
	}
	raw, hash, prefix, err := GenerateAPIKey()
	if err != nil {
		return nil, err
	}
	k := &domain.APIKey{
		ProjectID:    in.ProjectID,
		Hash:         hash,
		Prefix:       prefix,
		Allowlist:    in.Allowlist,
		BodyCap:      in.BodyCap,
		RateLimitRPS: in.RateLimitRPS,
		RedactRules:  in.RedactRules,
		Status:       "active",
		Description:  in.Description,
	}
	if err := a.keys.Create(ctx, k); err != nil {
		return nil, err
	}
	return &CreateAPIKeyOutput{Key: k, Raw: raw}, nil
}

func (a *APIKeys) List(ctx context.Context, projectID, ownerID uint64) ([]*domain.APIKey, error) {
	if _, err := a.projects.Get(ctx, projectID, ownerID); err != nil {
		return nil, err
	}
	return a.keys.List(ctx, projectID)
}

func (a *APIKeys) Get(ctx context.Context, projectID, ownerID, keyID uint64) (*domain.APIKey, error) {
	if _, err := a.projects.Get(ctx, projectID, ownerID); err != nil {
		return nil, err
	}
	return a.keys.Get(ctx, keyID, projectID)
}

type UpdateAPIKeyInput struct {
	ProjectID    uint64
	OwnerID      uint64
	KeyID        uint64
	Allowlist    []string
	BodyCap      int
	RateLimitRPS int
	RedactRules  []byte
	Description  string
}

func (a *APIKeys) Update(ctx context.Context, in UpdateAPIKeyInput) (*domain.APIKey, error) {
	if _, err := a.projects.Get(ctx, in.ProjectID, in.OwnerID); err != nil {
		return nil, err
	}
	k := &domain.APIKey{
		ID:           in.KeyID,
		ProjectID:    in.ProjectID,
		Allowlist:    in.Allowlist,
		BodyCap:      in.BodyCap,
		RateLimitRPS: in.RateLimitRPS,
		RedactRules:  in.RedactRules,
		Description:  in.Description,
	}
	if err := a.keys.Update(ctx, k); err != nil {
		return nil, err
	}
	return a.keys.Get(ctx, in.KeyID, in.ProjectID)
}

func (a *APIKeys) Revoke(ctx context.Context, projectID, ownerID, keyID uint64) error {
	if _, err := a.projects.Get(ctx, projectID, ownerID); err != nil {
		return err
	}
	return a.keys.Revoke(ctx, keyID, projectID)
}
