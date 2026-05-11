package usecase

import (
	"context"
	"time"

	"echoproxy/stats-api/internal/domain"
)

type Queries struct{ repo domain.Repository }

func NewQueries(repo domain.Repository) *Queries { return &Queries{repo: repo} }

func (q *Queries) ListLogs(ctx context.Context, f domain.LogsFilter) ([]domain.LogEvent, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	if f.From.IsZero() {
		f.From = time.Now().Add(-1 * time.Hour)
	}
	if f.To.IsZero() {
		f.To = time.Now().Add(time.Minute)
	}
	return q.repo.ListLogs(ctx, f)
}

func (q *Queries) GetLog(ctx context.Context, projectID uint64, eventID string) (*domain.LogEvent, error) {
	return q.repo.GetLog(ctx, projectID, eventID)
}

func (q *Queries) MinuteMetrics(ctx context.Context, projectID, apiKeyID uint64, from, to time.Time) ([]domain.MinuteMetric, error) {
	if from.IsZero() {
		from = time.Now().Add(-1 * time.Hour)
	}
	if to.IsZero() {
		to = time.Now()
	}
	return q.repo.MinuteMetrics(ctx, projectID, apiKeyID, from, to)
}

func (q *Queries) TopPaths(ctx context.Context, projectID uint64, from, to time.Time, limit int) ([]domain.TopPath, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	return q.repo.TopPaths(ctx, projectID, from, to, limit)
}

// chooseBucket picks a sensible bucket size (in seconds) for the requested
// range so the time series always has 30-200 points regardless of duration.
func chooseBucket(from, to time.Time) uint32 {
	d := to.Sub(from)
	switch {
	case d <= 30*time.Minute:
		return 30
	case d <= 2*time.Hour:
		return 60
	case d <= 6*time.Hour:
		return 5 * 60
	case d <= 24*time.Hour:
		return 15 * 60
	case d <= 7*24*time.Hour:
		return 60 * 60
	default:
		return 6 * 60 * 60
	}
}

func (q *Queries) defaultRange(f *domain.AnalyticsFilter) {
	if f.To.IsZero() {
		f.To = time.Now()
	}
	if f.From.IsZero() {
		f.From = f.To.Add(-1 * time.Hour)
	}
	if f.BucketSec == 0 {
		f.BucketSec = chooseBucket(f.From, f.To)
	}
}

func (q *Queries) TimeSeries(ctx context.Context, f domain.AnalyticsFilter) ([]domain.TimeBucket, error) {
	q.defaultRange(&f)
	return q.repo.TimeSeries(ctx, f)
}

func (q *Queries) StatusDistribution(ctx context.Context, f domain.AnalyticsFilter) ([]domain.Bucket, error) {
	q.defaultRange(&f)
	return q.repo.StatusDistribution(ctx, f)
}

func (q *Queries) MethodDistribution(ctx context.Context, f domain.AnalyticsFilter) ([]domain.Bucket, error) {
	q.defaultRange(&f)
	return q.repo.MethodDistribution(ctx, f)
}

func (q *Queries) HostDistribution(ctx context.Context, f domain.AnalyticsFilter) ([]domain.Bucket, error) {
	q.defaultRange(&f)
	return q.repo.HostDistribution(ctx, f)
}

func (q *Queries) EndpointStats(ctx context.Context, f domain.AnalyticsFilter, limit int) ([]domain.EndpointStat, error) {
	q.defaultRange(&f)
	return q.repo.EndpointStats(ctx, f, limit)
}
