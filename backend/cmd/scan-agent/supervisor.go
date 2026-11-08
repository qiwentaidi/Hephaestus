package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"hephaestus-backend/pkg/agentruntime"
	"hephaestus-backend/pkg/agentupdate"
	"hephaestus-backend/pkg/config"
	"hephaestus-backend/pkg/database"
)

func runSupervisor(args []string) error {
	flags := flag.NewFlagSet("supervise", flag.ContinueOnError)
	binary := flags.String("binary", "", "installed worker executable")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *binary == "" || flags.NArg() != 0 {
		return fmt.Errorf("supervise requires --binary PATH")
	}
	changeWorkDirToExecutableDir()
	if _, err := config.LoadConfig(); err != nil {
		return err
	}
	db, err := database.GetDB()
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	host, err := agentruntime.NodeHostIdentity(ctx)
	if err != nil {
		return err
	}
	base := strings.TrimSpace(os.Getenv("HEPHAESTUS_AGENT_RELEASE_BASE"))
	if base == "" {
		if data, e := os.ReadFile(filepath.Join(filepath.Dir(*binary), ".release-base")); e == nil {
			base = strings.TrimSpace(string(data))
		}
	}
	if base == "" {
		base = "https://github.com/qiwentaidi/Hephaestus-Agent/releases/download"
	}
	return (&agentupdate.Supervisor{DB: db, Binary: *binary, HostID: host, ReleaseBase: base, ResolveTarget: func(ctx context.Context) (string, error) {
		return agentupdate.ServerTarget(ctx, loadAgentServerAddress())
	}}).Run(ctx)
}
