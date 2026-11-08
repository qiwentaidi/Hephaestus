package nuclei

import (
	"context"
	"testing"

	"github.com/projectdiscovery/nuclei/v3/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestCreateEphemeralObjectsDoesNotSetTypedNilHostErrorsCache(t *testing.T) {
	baseOpts := types.DefaultOptions()
	scanOpts := baseOpts.Copy()
	scanOpts.NoHostErrors = true
	scanOpts.MaxHostError = 0

	base := &NucleiEngine{
		opts:   baseOpts,
		Logger: baseOpts.Logger,
	}

	unsafeOpts, err := createEphemeralObjects(context.Background(), base, scanOpts)
	require.NoError(t, err)
	t.Cleanup(func() {
		closeEphemeralObjects(unsafeOpts)
	})

	require.Nil(t, unsafeOpts.executerOpts.HostErrorsCache)
}
