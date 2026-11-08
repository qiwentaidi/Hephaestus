package webscan

import (
	"context"
	"testing"

	nuclei "github.com/projectdiscovery/nuclei/v3/lib"
	syncutil "github.com/projectdiscovery/utils/sync"
)

func TestIsBenignNucleiSkipError(t *testing.T) {
	if !isBenignNucleiSkipError(nuclei.ErrNoTemplatesAvailable) {
		t.Fatal("expected no-templates error to be treated as a benign skip")
	}
}

func TestXxx(t *testing.T) {
	ctx := context.Background()
	// when running nuclei in parallel for first time it is a good practice to make sure
	// templates exists first

	// create nuclei engine with options
	ne, err := nuclei.NewThreadSafeNucleiEngineCtx(ctx)
	if err != nil {
		panic(err)
	}
	// setup sizedWaitgroup to handle concurrency
	sg, err := syncutil.New(syncutil.WithSize(10))
	if err != nil {
		panic(err)
	}

	// scan 1 = run dns templates on scanme.sh
	sg.Add()
	go func() {
		defer sg.Done()
		err = ne.ExecuteNucleiWithOpts([]string{"scanme.sh"},
			nuclei.WithTemplateFilters(nuclei.TemplateFilters{ProtocolTypes: "dns"}),
			// nuclei.WithProxy([]string{"http://127.0.0.1:8080"}, false),
			nuclei.WithInteractshOptions(nuclei.InteractshOpts{
				Authorization: "",
				ServerURL:     "",
			}),
			nuclei.WithHeaders([]string{"X-Bug-Bounty: pdteam"}),
			nuclei.EnablePassiveMode(),
		)
		if err != nil {
			panic(err)
		}
	}()

	// wait for all scans to finish
	sg.Wait()
	defer ne.Close()
}
