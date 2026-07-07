package detonate

import (
	"context"
	"sync"
	"time"
)

var _ Runner = (*FakeRunner)(nil)

// Runner abstracts every VM/network side effect a detonation run needs:
// checking/cloning the immutable golden, attaching an isolated network,
// injecting a sample offline, booting for a bounded time, collecting
// artifacts, and destroying the clone. The lifecycle logic (cmd/detonate)
// and all tests in this package drive it through this interface so no test
// ever needs a real Tart VM. TartRunner (tart.go) is the real
// implementation; FakeRunner below is for tests.
type Runner interface {
	GoldenExists(ctx context.Context, golden string) (bool, error)
	Clone(ctx context.Context, golden, run string) error
	ConfigureIsolatedNet(ctx context.Context, run, gw string) (NetAttachment, error)
	InjectOffline(ctx context.Context, run, samplePath string) error
	Run(ctx context.Context, run string, timeout time.Duration) error
	Collect(ctx context.Context, run, destDir string) ([]string, error)
	PoweredOff(ctx context.Context, run string) (bool, error)
	Destroy(ctx context.Context, run string) error
}

// Call records one Runner method invocation, in order, for test assertions.
type Call struct {
	Method string
	Args   []string
}

// FakeRunner is an in-memory Runner for tests: it records every call in Log
// and returns configurable canned results/errors, mirroring
// pkg/vmexec.FakeExecutor's call-log + SetX setter pattern. Unconfigured
// calls return the zero value and no error.
type FakeRunner struct {
	mu  sync.Mutex
	Log []Call

	goldenExists  map[string]bool
	goldenErr     map[string]error
	cloneErr      map[string]error
	netAttachment map[string]NetAttachment
	netErr        map[string]error
	injectErr     map[string]error
	runErr        map[string]error
	collectFiles  map[string][]string
	collectErr    map[string]error
	poweredOff    map[string]bool
	poweredOffErr map[string]error
	destroyErr    map[string]error
}

func NewFakeRunner() *FakeRunner {
	return &FakeRunner{
		goldenExists:  make(map[string]bool),
		goldenErr:     make(map[string]error),
		cloneErr:      make(map[string]error),
		netAttachment: make(map[string]NetAttachment),
		netErr:        make(map[string]error),
		injectErr:     make(map[string]error),
		runErr:        make(map[string]error),
		collectFiles:  make(map[string][]string),
		collectErr:    make(map[string]error),
		poweredOff:    make(map[string]bool),
		poweredOffErr: make(map[string]error),
		destroyErr:    make(map[string]error),
	}
}

func (f *FakeRunner) record(method string, args ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Log = append(f.Log, Call{Method: method, Args: args})
}

// LastCall returns the most recently recorded call, or nil if none.
func (f *FakeRunner) LastCall() *Call {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.Log) == 0 {
		return nil
	}
	return &f.Log[len(f.Log)-1]
}

func (f *FakeRunner) SetGoldenExists(golden string, exists bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.goldenExists[golden] = exists
}

func (f *FakeRunner) SetGoldenExistsErr(golden string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.goldenErr[golden] = err
}

func (f *FakeRunner) SetCloneErr(run string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cloneErr[run] = err
}

func (f *FakeRunner) SetNetAttachment(run string, n NetAttachment) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.netAttachment[run] = n
}

func (f *FakeRunner) SetConfigureNetErr(run string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.netErr[run] = err
}

func (f *FakeRunner) SetInjectErr(run string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.injectErr[run] = err
}

func (f *FakeRunner) SetRunErr(run string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runErr[run] = err
}

func (f *FakeRunner) SetCollectFiles(run string, files []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.collectFiles[run] = files
}

func (f *FakeRunner) SetCollectErr(run string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.collectErr[run] = err
}

// SetCollectResult configures Collect to return both a partial file list and
// an error, mirroring TartRunner.Collect's real (alreadyCopied, err) contract
// when it fails mid-loop after copying some artifacts.
func (f *FakeRunner) SetCollectResult(run string, files []string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.collectFiles[run] = files
	f.collectErr[run] = err
}

func (f *FakeRunner) SetPoweredOff(run string, off bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.poweredOff[run] = off
}

func (f *FakeRunner) SetPoweredOffErr(run string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.poweredOffErr[run] = err
}

func (f *FakeRunner) SetDestroyErr(run string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.destroyErr[run] = err
}

func (f *FakeRunner) GoldenExists(_ context.Context, golden string) (bool, error) {
	f.record("GoldenExists", golden)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.goldenErr[golden]; err != nil {
		return false, err
	}
	return f.goldenExists[golden], nil
}

func (f *FakeRunner) Clone(_ context.Context, golden, run string) error {
	f.record("Clone", golden, run)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cloneErr[run]
}

func (f *FakeRunner) ConfigureIsolatedNet(_ context.Context, run, gw string) (NetAttachment, error) {
	f.record("ConfigureIsolatedNet", run, gw)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.netErr[run]; err != nil {
		return NetAttachment{}, err
	}
	if n, ok := f.netAttachment[run]; ok {
		return n, nil
	}
	return NetAttachment{Mode: "isolated", HasUplink: false}, nil
}

func (f *FakeRunner) InjectOffline(_ context.Context, run, samplePath string) error {
	f.record("InjectOffline", run, samplePath)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.injectErr[run]
}

func (f *FakeRunner) Run(_ context.Context, run string, timeout time.Duration) error {
	f.record("Run", run, timeout.String())
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.runErr[run]
}

func (f *FakeRunner) Collect(_ context.Context, run, destDir string) ([]string, error) {
	f.record("Collect", run, destDir)
	f.mu.Lock()
	defer f.mu.Unlock()
	// Return files and err together, like TartRunner.Collect: a mid-loop
	// failure there still returns the artifacts already copied.
	return f.collectFiles[run], f.collectErr[run]
}

func (f *FakeRunner) PoweredOff(_ context.Context, run string) (bool, error) {
	f.record("PoweredOff", run)
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.poweredOffErr[run]; err != nil {
		return false, err
	}
	return f.poweredOff[run], nil
}

func (f *FakeRunner) Destroy(_ context.Context, run string) error {
	f.record("Destroy", run)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.destroyErr[run]
}
