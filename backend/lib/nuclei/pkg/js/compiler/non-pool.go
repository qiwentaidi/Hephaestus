package compiler

import (
	"context"
	"sync"

	"github.com/Mzack9999/goja"
	syncutil "github.com/projectdiscovery/utils/sync"
)

var (
	ephemeraljsc    *syncutil.AdaptiveWaitGroup
	lazyFixedSgInit = sync.OnceFunc(func() {
		ephemeraljsc, _ = syncutil.New(syncutil.WithSize(NonPoolingVMConcurrency))
	})
)

func executeWithoutPooling(p *goja.Program, args *ExecuteArgs, opts *ExecuteOptions) (result goja.Value, err error) {
	lazyFixedSgInit()
	ctx := context.Background()
	if opts != nil && opts.Context != nil {
		ctx = opts.Context
	}
	if err := ephemeraljsc.AddWithContext(ctx); err != nil {
		return nil, err
	}
	defer ephemeraljsc.Done()
	runtime := createNewRuntime()
	return executeWithRuntime(runtime, p, args, opts)
}
