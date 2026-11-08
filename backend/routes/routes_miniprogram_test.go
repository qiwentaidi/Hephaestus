package routes

import (
	"testing"

	appconfig "hephaestus-backend/pkg/config"

	"github.com/gin-gonic/gin"
)

func TestMiniProgramDecompileRoutesAreDesktopOnly(t *testing.T) {
	t.Cleanup(func() {
		appconfig.SetRuntimeMode(appconfig.RuntimeModeServer)
	})

	for _, test := range []struct {
		name      string
		runtime   string
		available bool
	}{
		{name: "server", runtime: appconfig.RuntimeModeServer, available: false},
		{name: "desktop", runtime: appconfig.RuntimeModeDesktop, available: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			appconfig.SetRuntimeMode(test.runtime)
			router := gin.New()
			SetupRoutes(router)

			for _, route := range router.Routes() {
				if route.Path != "/api/v1/tools/mini-program-decompile/settings" {
					continue
				}
				if route.Method == "GET" {
					if !test.available {
						t.Fatal("mini-program decompile route must not be registered in server mode")
					}
					return
				}
			}

			if test.available {
				t.Fatal("mini-program decompile route must be registered in desktop mode")
			}
		})
	}
}

func TestAIPentestRoutesAreRegistered(t *testing.T) {
	t.Cleanup(func() {
		appconfig.SetRuntimeMode(appconfig.RuntimeModeServer)
	})
	appconfig.SetRuntimeMode(appconfig.RuntimeModeServer)
	router := gin.New()
	SetupRoutes(router)

	expected := map[string]bool{
		"GET /api/v1/ai/model-options":                false,
		"GET /api/v1/ai/tools":                        false,
		"GET /api/v1/ai/strategy-packs":               false,
		"GET /api/v1/ai/nodes":                        false,
		"GET /api/v1/ai/projects":                     false,
		"POST /api/v1/ai/projects":                    false,
		"GET /api/v1/ai/projects/:id/blackboard":      false,
		"PATCH /api/v1/ai/projects/:id":               false,
		"DELETE /api/v1/ai/projects/:id":              false,
		"GET /api/v1/ai/sessions/bootstrap":           false,
		"GET /api/v1/ai/sessions":                     false,
		"POST /api/v1/ai/sessions":                    false,
		"GET /api/v1/ai/sessions/:id":                 false,
		"PATCH /api/v1/ai/sessions/:id":               false,
		"DELETE /api/v1/ai/sessions/:id":              false,
		"GET /api/v1/ai/sessions/:id/stream":          false,
		"POST /api/v1/ai/sessions/:id/message":        false,
		"POST /api/v1/ai/sessions/:id/message/stream": false,
		"POST /api/v1/ai/sessions/:id/cancel":         false,
		"POST /api/v1/ai/sessions/:id/action":         false,
		"GET /api/v1/ai/sessions/:id/blackboard":      false,
		"GET /api/v1/ai/sessions/:id/report":          false,
		"GET /api/v1/ai/findings":                     false,
		"GET /api/v1/ai/findings/management":          false,
	}
	for _, route := range router.Routes() {
		key := route.Method + " " + route.Path
		if _, ok := expected[key]; ok {
			expected[key] = true
		}
	}
	for route, found := range expected {
		if !found {
			t.Fatalf("AI pentest route not registered: %s", route)
		}
	}
	for _, route := range router.Routes() {
		switch route.Path {
		case "/api/v1/ai/roles", "/api/v1/ai/roles/capabilities", "/api/v1/ai/roles/:id", "/api/v1/ai/roles/preview", "/api/v1/ai/sessions/:id/role", "/api/v1/ai/agent-profiles":
			t.Fatalf("removed role endpoint is still registered: %s %s", route.Method, route.Path)
		}
	}
}
