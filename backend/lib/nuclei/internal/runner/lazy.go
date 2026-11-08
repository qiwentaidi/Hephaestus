package runner

import (
	"github.com/projectdiscovery/nuclei/v3/internal/engineconfig"
	"github.com/projectdiscovery/nuclei/v3/pkg/authprovider/authx"
	"github.com/projectdiscovery/nuclei/v3/pkg/catalog"
	"github.com/projectdiscovery/nuclei/v3/pkg/catalog/loader"
	"github.com/projectdiscovery/nuclei/v3/pkg/protocols"
	"github.com/projectdiscovery/nuclei/v3/pkg/types"
)

type AuthLazyFetchOptions = engineconfig.AuthLazyFetchOptions

func GetAuthTmplStore(opts *types.Options, catalog catalog.Catalog, execOpts *protocols.ExecutorOptions) (*loader.Store, error) {
	return engineconfig.GetAuthTmplStore(opts, catalog, execOpts)
}

func GetLazyAuthFetchCallback(opts *AuthLazyFetchOptions) authx.LazyFetchSecret {
	return engineconfig.GetLazyAuthFetchCallback(opts)
}
