package kafka

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"github.com/twmb/franz-go/pkg/kgo"
	"google.golang.org/protobuf/proto"

	"github.com/songhieu/EchoProxy/log-consumer/internal/domain"
	"github.com/songhieu/EchoProxy/log-consumer/internal/usecase"
	"github.com/songhieu/EchoProxy/pkg/event"
)

// Consumer drains a Kafka topic into the batcher. One consumer per process is
// fine for MVP; rebalance gives us natural sharding when scaled out.
type Consumer struct {
	client  *kgo.Client
	batcher *usecase.Batcher
	tickEvery time.Duration
	log     zerolog.Logger
}

type Config struct {
	Brokers   []string
	Topic     string
	Group     string
	TickEvery time.Duration
}

func NewConsumer(cfg Config, batcher *usecase.Batcher, log zerolog.Logger) (*Consumer, error) {
	if cfg.TickEvery <= 0 {
		cfg.TickEvery = 200 * time.Millisecond
	}
	cl, err := kgo.NewClient(
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumerGroup(cfg.Group),
		kgo.ConsumeTopics(cfg.Topic),
		kgo.DisableAutoCommit(),
	)
	if err != nil {
		return nil, fmt.Errorf("kafka new client: %w", err)
	}
	return &Consumer{client: cl, batcher: batcher, tickEvery: cfg.TickEvery, log: log}, nil
}

// Run drives a fetch goroutine and a flusher goroutine. PollFetches blocks
// indefinitely on franz-go, so we run the flush ticker on its own goroutine
// to avoid starving deadline-based flushes.
func (c *Consumer) Run(ctx context.Context) {
	var batchMu sync.Mutex
	flushDone := make(chan struct{})
	go func() {
		defer close(flushDone)
		tick := time.NewTicker(c.tickEvery)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				batchMu.Lock()
				_ = c.batcher.MaybeFlush(ctx)
				batchMu.Unlock()
				if err := c.client.CommitUncommittedOffsets(ctx); err != nil && ctx.Err() == nil {
					c.log.Error().Err(err).Msg("commit offsets")
				}
			}
		}
	}()

	for {
		fetches := c.client.PollFetches(ctx)
		if ctx.Err() != nil {
			break
		}
		if errs := fetches.Errors(); len(errs) > 0 {
			for _, e := range errs {
				if e.Err == context.Canceled {
					continue
				}
				c.log.Error().Err(e.Err).Str("topic", e.Topic).Int32("partition", e.Partition).Msg("kafka fetch error")
			}
		}
		fetches.EachRecord(func(rec *kgo.Record) {
			row, err := decode(rec.Value)
			if err != nil {
				c.log.Warn().Err(err).Msg("decode event")
				return
			}
			batchMu.Lock()
			_ = c.batcher.Add(ctx, row)
			batchMu.Unlock()
		})
	}
	<-flushDone
	batchMu.Lock()
	c.flushAndCommit(context.Background())
	batchMu.Unlock()
	c.client.Close()
}

func (c *Consumer) flushAndCommit(ctx context.Context) {
	if err := c.batcher.Flush(ctx); err != nil {
		c.log.Error().Err(err).Msg("final flush")
	}
	if err := c.client.CommitUncommittedOffsets(ctx); err != nil {
		c.log.Error().Err(err).Msg("final commit")
	}
}

func decode(b []byte) (domain.Row, error) {
	var ev event.HttpEvent
	if err := proto.Unmarshal(b, &ev); err != nil {
		return domain.Row{}, err
	}
	return domain.Row{
		TimestampMs:  ev.TimestampNs / 1_000_000,
		EventID:      ev.EventId,
		ProjectID:    ev.ProjectId,
		APIKeyID:     ev.ApiKeyId,
		Source:       ev.Source,
		Direction:    ev.Direction,
		SDKVersion:   ev.SdkVersion,
		Method:       ev.Method,
		Scheme:       ev.Scheme,
		Host:         ev.Host,
		Path:         ev.Path,
		Query:        ev.Query,
		Status:            uint16(ev.Status),
		LatencyMs:         ev.LatencyMs,
		UpstreamLatencyMs: ev.UpstreamLatencyMs,
		UpstreamTtfbMs:    ev.UpstreamTtfbMs,
		ReqSize:           ev.ReqSize,
		ResSize:           ev.ResSize,
		ReqHeaders:   nilSafeMap(ev.ReqHeaders),
		ResHeaders:   nilSafeMap(ev.ResHeaders),
		ReqBody:      string(ev.ReqBody),
		ResBody:      string(ev.ResBody),
		ReqTruncated: boolToU8(ev.ReqBodyTruncated),
		ResTruncated: boolToU8(ev.ResBodyTruncated),
		ClientIP:     ev.ClientIp,
		UserAgent:    ev.UserAgent,
		TraceID:      ev.TraceId,
		Attributes:   nilSafeMap(ev.Attributes),
		Error:        ev.Error,
	}, nil
}

func nilSafeMap(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func boolToU8(b bool) uint8 {
	if b {
		return 1
	}
	return 0
}
