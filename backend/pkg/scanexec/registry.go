package scanexec

import (
	"context"
	"sort"
	"strings"

	"hephaestus-backend/pkg/dirsearch"
	"hephaestus-backend/pkg/jsscan"
	"hephaestus-backend/pkg/portscan"
	"hephaestus-backend/pkg/subdomain"
	"hephaestus-backend/pkg/webscan"
)

type Handler func(context.Context, string, []string, map[string]interface{}) error

var Handlers = map[string]Handler{
	"webscan":   webscan.ExecuteLocalScanWithContext,
	"portscan":  portscan.ExecutePortScanWithContext,
	"crack":     portscan.ExecuteCrackWithContext,
	"dirscan":   dirsearch.ExecuteDirScanWithContext,
	"jsscan":    jsscan.ExecuteJSScanWithContext,
	"subdomain": subdomain.ExecuteSubdomainScanWithContext,
}

func GetHandler(scanType string) (Handler, bool) {
	handler, ok := Handlers[strings.ToLower(strings.TrimSpace(scanType))]
	return handler, ok
}

func GetSupportedScanTypes() []string {
	types := make([]string, 0, len(Handlers))
	for key := range Handlers {
		types = append(types, key)
	}
	sort.Strings(types)
	return types
}
