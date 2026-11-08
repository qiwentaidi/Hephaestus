package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type AgentWorkerConfig struct {
	AIEnabled, ScanEnabled         bool
	AIConcurrency, ScanConcurrency int
}

// Environment overrides retain existing deployment compatibility. YAML is
// shared by embedded and standalone nodes; changes take effect on restart.
func ResolveAgentWorkers(agent AgentConfig, embedded bool, scanDefault int) (AgentWorkerConfig, error) {
	roles := agent.Roles
	if len(roles) == 0 {
		roles = []string{"scan"}
		if embedded {
			roles = []string{"ai", "scan"}
		}
	}
	if value := strings.TrimSpace(os.Getenv("AGENT_ROLES")); value != "" {
		roles = strings.Split(value, ",")
	}
	var result AgentWorkerConfig
	for _, role := range roles {
		switch strings.TrimSpace(role) {
		case "ai":
			result.AIEnabled = true
		case "scan":
			result.ScanEnabled = true
		default:
			return result, fmt.Errorf("unsupported Agent role %q", role)
		}
	}
	// ARTEX default: three workers plus one independent planner slot.
	aiDefault := 4
	if scanDefault <= 0 {
		scanDefault = 3
	}
	capacity := func(key string, value, fallback int) (int, error) {
		if raw := strings.TrimSpace(os.Getenv(key)); raw != "" {
			parsed, err := strconv.Atoi(raw)
			if err != nil || parsed <= 0 {
				return 0, fmt.Errorf("%s must be a positive integer", key)
			}
			return parsed, nil
		}
		if value < 0 {
			return 0, fmt.Errorf("Agent concurrency must be positive")
		}
		if value == 0 {
			value = fallback
		}
		return value, nil
	}
	var err error
	if result.AIConcurrency, err = capacity("AGENT_AI_CONCURRENCY", agent.AIConcurrency, aiDefault); err != nil {
		return result, err
	}
	result.ScanConcurrency, err = capacity("AGENT_CONCURRENCY", agent.ScanConcurrency, scanDefault)
	return result, err
}

// PersistAgentID preserves unrelated YAML, comments and existing permissions.
// The machine registration chooses the identity; changing configuration paths
// or manually editing an ID cannot register another node for the same host.
func PersistAgentID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("Agent ID cannot be empty")
	}
	mu.Lock()
	defer mu.Unlock()
	path := GetResolvedConfigPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var document yaml.Node
	if err = yaml.Unmarshal(raw, &document); err != nil {
		return err
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return fmt.Errorf("invalid Agent configuration document")
	}
	root := document.Content[0]
	child := func(mapping *yaml.Node, key string) *yaml.Node {
		for i := 0; i < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value == key {
				return mapping.Content[i+1]
			}
		}
		value := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str"}
		mapping.Content = append(mapping.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, value)
		return value
	}
	agent := child(root, "agent")
	if agent.Kind == yaml.ScalarNode && agent.Value == "" {
		agent.Kind = yaml.MappingNode
		agent.Tag = "!!map"
	}
	if agent.Kind != yaml.MappingNode {
		return fmt.Errorf("agent must be a mapping")
	}
	value := child(agent, "id")
	if value.Value == id {
		return nil
	}
	value.Kind = yaml.ScalarNode
	value.Tag = "!!str"
	value.Value = id
	raw, err = yaml.Marshal(&document)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".agent-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if err = file.Chmod(info.Mode().Perm()); err != nil {
		file.Close()
		return err
	}
	if _, err = file.Write(raw); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
