package pipe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/github/go-kvp"
	"github.com/github/go-log"
)

const memoryPollInterval = time.Second

// ErrMemoryLimitExceeded is the error that will be used to kill a process, if
// necessary, from MemoryLimit.
var ErrMemoryLimitExceeded = errors.New("memory limit exceeded")

// LimitableStage is the superset of Stage that must be implemented by stages
// passed to MemoryLimit and MemoryObsever.
type LimitableStage interface {
	Stage

	GetRSS(context.Context) (uint64, error)
	Kill(error)
}

// MemoryLimit watches the memory usage of the stage and stops it if it
// exceeds the given limit.
func MemoryLimit(stage Stage, byteLimit uint64, logger log.FieldLogger) Stage {
	logger = logger.With(
		kvp.String("fn", "pipe.MemoryLimit"),
		kvp.String("stage", stage.Name()))

	limitableStage, ok := stage.(LimitableStage)
	if !ok {
		logger.Error("invalid pipe.MemoryLimit usage",
			kvp.String("stage_type", fmt.Sprintf("%T", stage)))
		return stage
	}

	return &memoryWatchStage{
		nameSuffix: " with memory limit",
		stage:      limitableStage,
		watch:      killAtLimit(byteLimit, logger),
	}
}

func killAtLimit(byteLimit uint64, logger log.FieldLogger) memoryWatchFunc {
	return func(ctx context.Context, stage LimitableStage) {
		var consecutiveErrors int

		t := time.NewTicker(memoryPollInterval)
		defer t.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				rss, err := stage.GetRSS(ctx)
				if err != nil {
					consecutiveErrors++
					if consecutiveErrors >= 2 {
						logger.Error("error getting RSS", kvp.Err(err))
					}
					continue
				}
				consecutiveErrors = 0
				if rss < byteLimit {
					continue
				}
				logger.Error("stage exceeded allowed memory use",
					kvp.Uint64("limit", byteLimit),
					kvp.Uint64("used", rss),
				)
				stage.Kill(ErrMemoryLimitExceeded)
				return
			}
		}
	}
}

// MemoryObserver watches memory use of the stage and logs the maximum
// value when the stage exits.
func MemoryObserver(stage Stage, logger log.FieldLogger) Stage {
	logger = logger.With(
		kvp.String("fn", "pipe.MemoryObserver"),
		kvp.String("stage", stage.Name()))

	limitableStage, ok := stage.(LimitableStage)
	if !ok {
		logger.Error("invalid pipe.MemoryObserver usage",
			kvp.String("stage_type", fmt.Sprintf("%T", stage)))
		return stage
	}

	return &memoryWatchStage{
		stage: limitableStage,
		watch: logMaxRSS(logger),
	}
}

func logMaxRSS(logger log.FieldLogger) memoryWatchFunc {
	return func(ctx context.Context, stage LimitableStage) {
		var (
			maxRSS                             uint64
			samples, errors, consecutiveErrors int
		)

		t := time.NewTicker(memoryPollInterval)
		defer t.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.Info("peak memory usage",
					kvp.Uint64("max_rss_bytes", maxRSS),
					kvp.Int("samples", samples),
					kvp.Int("errors", errors),
				)
				return
			case <-t.C:
				rss, err := stage.GetRSS(ctx)
				if err != nil {
					errors++
					consecutiveErrors++
					if consecutiveErrors == 2 {
						logger.Error("error getting RSS", kvp.Err(err))
					}
					// don't log any more errors until we get rss successfully.
					continue
				}

				consecutiveErrors = 0
				samples++
				if rss > maxRSS {
					maxRSS = rss
				}
			}
		}
	}
}

type memoryWatchStage struct {
	nameSuffix string
	stage      LimitableStage
	watch      memoryWatchFunc
}

type memoryWatchFunc func(context.Context, LimitableStage)

var _ LimitableStage = (*memoryWatchStage)(nil)

func (m *memoryWatchStage) Name() string {
	return m.stage.Name() + m.nameSuffix
}

func (m *memoryWatchStage) Start(ctx context.Context, env Env, stdin io.ReadCloser) (io.ReadCloser, error) {
	io, err := m.stage.Start(ctx, env, stdin)
	if err != nil {
		return nil, err
	}
	go m.watch(ctx, m.stage)
	return io, nil
}

func (m *memoryWatchStage) Wait() error {
	return m.stage.Wait()
}

func (m *memoryWatchStage) GetRSS(ctx context.Context) (uint64, error) {
	return m.stage.GetRSS(ctx)
}

func (m *memoryWatchStage) Kill(err error) {
	m.stage.Kill(err)
}
