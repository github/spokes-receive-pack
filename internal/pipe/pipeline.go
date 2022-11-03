package pipe

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync/atomic"

	"github.com/github/go-kvp"
	"github.com/github/go-log"
	"github.com/github/go-trace"
)

// Env represents the environment that a pipeline stage should run in.
// It is passed to `Stage.Start()`.
type Env struct {
	// The directory in which external commands should be executed by
	// default.
	Dir string

	// Vars are extra environment variables. These will override any
	// environment variables that would be inherited from the current
	// process.
	Vars []AppendVars
}

type AppendVars func(context.Context, []EnvVar) []EnvVar

// EnvVar represents an environment variable that will be provided to any child
// process spawned in this pipeline. Only one of ValueFunc or Value should be
// provided.
type EnvVar struct {
	// The name of the environment variable.
	Key string
	// The value, if static.
	Value string
}

type ContextValueFunc func(context.Context) (string, bool)

type ContextValuesFunc func(context.Context) []EnvVar

// Pipeline represents a Unix-like pipe that can include multiple
// stages, including external processes but also and stages written in
// Go.
type Pipeline struct {
	env Env

	stdin  io.Reader
	stdout io.WriteCloser
	stages []Stage
	cancel func()

	// Atomically written and read value, nonzero if the pipeline has
	// been started. This is only used for lifecycle sanity checks but
	// does not guarantee that clients are using the class correctly.
	started uint32

	span   trace.Span
	logger log.FieldLogger
}

type nopWriteCloser struct {
	io.Writer
}

func (w nopWriteCloser) Close() error {
	return nil
}

type NewPipeFn func(dir string, opts ...Option) *Pipeline

// NewPipeline returns the Pipeline struct with all of the required
// fields set. The directory must be non-empty if any Git commands are
// to be run.
func New(dir string, opts ...Option) *Pipeline {
	p := &Pipeline{
		env: Env{
			Dir: dir,
		},
		logger: log.NullLogger,
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Option is a type alias for Pipeline functional options.
type Option func(*Pipeline)

// WithStdin assigns stdin to the first command in the pipeline.
func WithStdin(stdin io.Reader) Option {
	return func(p *Pipeline) {
		p.stdin = stdin
	}
}

// WithStdout assigns stdout to the last command in the pipeline.
func WithStdout(stdout io.Writer) Option {
	return func(p *Pipeline) {
		p.stdout = nopWriteCloser{stdout}
	}
}

// WithStdoutCloser assigns stdout to the last command in the
// pipeline, and closes stdout when it's done.
func WithStdoutCloser(stdout io.WriteCloser) Option {
	return func(p *Pipeline) {
		p.stdout = stdout
	}
}

// WithLogger sets a logger for the pipeline. Setting one will emit
// stderr of each process to the logs.
func WithLogger(logger log.FieldLogger) Option {
	return func(p *Pipeline) {
		p.logger = logger.With(kvp.Bool("pipeline", true))
	}
}

// WithEnvVar appends an environment variable for the pipeline.
func WithEnvVar(key, value string) Option {
	return func(p *Pipeline) {
		p.env.Vars = append(p.env.Vars, func(_ context.Context, vars []EnvVar) []EnvVar {
			return append(vars, EnvVar{Key: key, Value: value})
		})
	}
}

// WithEnvVars appends several environment variable for the pipeline.
func WithEnvVars(b []EnvVar) Option {
	return func(p *Pipeline) {
		p.env.Vars = append(p.env.Vars, func(_ context.Context, a []EnvVar) []EnvVar {
			return append(a, b...)
		})
	}
}

// WithEnvVarFunc appends a context-based environment variable for the pipeline.
func WithEnvVarFunc(key string, valueFunc ContextValueFunc) Option {
	return func(p *Pipeline) {
		p.env.Vars = append(p.env.Vars, func(ctx context.Context, vars []EnvVar) []EnvVar {
			if val, ok := valueFunc(ctx); ok {
				return append(vars, EnvVar{Key: key, Value: val})
			}
			return vars
		})
	}
}

// WithEnvVarsFunc appends several context-based environment variables for the pipeline.
func WithEnvVarsFunc(valuesFunc ContextValuesFunc) Option {
	return func(p *Pipeline) {
		p.env.Vars = append(p.env.Vars, func(ctx context.Context, vars []EnvVar) []EnvVar {
			return append(vars, valuesFunc(ctx)...)
		})
	}
}

func (p *Pipeline) hasStarted() bool {
	return atomic.LoadUint32(&p.started) != 0
}

// Add appends one or more stages to the pipeline.
func (p *Pipeline) Add(stages ...Stage) {
	if p.hasStarted() {
		panic("attempt to modify a pipeline that has already started")
	}

	p.stages = append(p.stages, stages...)
}

// AddWithIgnoredError appends one or more stages that are ignoring
// the passed in error to the pipeline.
func (p *Pipeline) AddWithIgnoredError(em ErrorMatcher, stages ...Stage) {
	if p.hasStarted() {
		panic("attempt to modify a pipeline that has already started")
	}

	for _, stage := range stages {
		p.stages = append(p.stages, IgnoreError(stage, em))
	}
}

// Start starts the commands in the pipeline. If `Start()` exits
// without an error, `Wait()` must also be called, to allow all
// resources to be freed.
func (p *Pipeline) Start(ctx context.Context) error {
	if p.hasStarted() {
		panic("attempt to start a pipeline that has already started")
	}

	atomic.StoreUint32(&p.started, 1)
	ctx, p.cancel = context.WithCancel(ctx)
	ctx, p.span = trace.ChildSpan(ctx)

	var nextStdin io.ReadCloser
	if p.stdin != nil {
		// We don't want the first stage to actually close this, and
		// it's not even an `io.ReadCloser`, so fake it:
		nextStdin = io.NopCloser(p.stdin)
	}

	for i, s := range p.stages {
		var err error
		stdout, err := s.Start(ctx, p.env, nextStdin)
		if err != nil {
			// Close the pipe that the previous stage was writing to.
			// That should cause it to exit even if it's not minding
			// its context.
			if nextStdin != nil {
				nextStdin.Close()
			}

			// Kill and wait for any stages that have been started
			// already to finish:
			p.cancel()
			for _, s := range p.stages[:i] {
				_ = s.Wait()
			}
			p.logger.Error(
				"failed to start pipeline stage",
				kvp.String("command", s.Name()), kvp.Err(err),
			)
			return fmt.Errorf("starting pipeline stage %q: %w", s.Name(), err)
		}
		nextStdin = stdout
	}

	// If the pipeline was configured with a `stdout`, add a synthetic
	// stage to copy the last stage's stdout to that writer:
	if p.stdout != nil {
		c := newIOCopier(p.stdout)
		p.stages = append(p.stages, c)
		// `ioCopier.Start()` never fails:
		_, _ = c.Start(ctx, p.env, nextStdin)
	}

	return nil
}

func (p *Pipeline) Output(ctx context.Context) ([]byte, error) {
	var buf bytes.Buffer
	p.stdout = nopWriteCloser{&buf}
	err := p.Run(ctx)
	return buf.Bytes(), err
}

// Wait waits for each stage in the pipeline to exit.
func (p *Pipeline) Wait() error {
	if !p.hasStarted() {
		panic("unable to wait on a pipeline that has not started")
	}

	// Make sure that all of the cleanup eventually happens:
	defer p.cancel()
	defer p.span.Finish()

	var earliestStageErr error
	var earliestFailedStage Stage

	for i := len(p.stages) - 1; i >= 0; i-- {
		s := p.stages[i]
		err := s.Wait()
		if err != nil {
			// Overwrite any existing values here so that we end up
			// retaining the last error that we see; i.e., the error
			// that happened earliest in the pipeline.
			earliestStageErr = err
			earliestFailedStage = s
		}
	}

	if earliestStageErr != nil {
		kvps := []kvp.Field{
			kvp.String("command", earliestFailedStage.Name()),
			kvp.Err(earliestStageErr),
		}
		if err, ok := earliestStageErr.(*exec.ExitError); ok && len(err.Stderr) > 0 {
			kvps = append(kvps, kvp.String("stderr", string(err.Stderr)))
		}

		p.logger.Error("command failed", kvps...)

		return p.span.WithError(fmt.Errorf("%s: %w", earliestFailedStage.Name(), earliestStageErr))
	}

	return nil
}

// Run starts and waits for the commands in the pipeline.
func (p *Pipeline) Run(ctx context.Context) error {
	if err := p.Start(ctx); err != nil {
		return err
	}

	return p.Wait()
}
