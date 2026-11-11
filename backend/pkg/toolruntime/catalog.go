// Package toolruntime adapts Norma's tools without owning an agent loop or a
// database. Both dialogue and orchestration consume this same catalog/runtime.
package toolruntime

import actool "github.com/Autumn-27/norma/tool"

func Available(name string) bool {
	switch name {
	case "Monitor", "TaskList", "TaskOutput", "TaskStop":
		return !actool.BackgroundTasksDisabled()
	case "shell_open", "shell_send", "shell_read", "shell_close", "shell_list":
		return !actool.InteractiveShellDisabled()
	}
	return true
}

func Tools() []actool.CoreTool {
	t := actool.DefaultTools()
	t = append(t, actool.NewTaskOutput(), actool.NewTaskList(), actool.NewTaskStop(), actool.NewMonitor())
	return append(t, actool.ShellSessionTools()...)
}

func Names() []string {
	var out []string
	for _, t := range Tools() {
		out = append(out, t.Name())
	}
	return out
}

func Lookup(name string) (actool.CoreTool, bool) {
	for _, t := range Tools() {
		if t.Name() == name {
			return t, true
		}
	}
	return nil, false
}

func IsCommand(name string) bool {
	return name == "Bash" || name == "Monitor" || name == "shell_open" || name == "shell_send"
}

// MigrateDisabled preserves the old command permission across every new command
// carrier. Existing task inspection/stop tools remain usable for cleanup.
func MigrateDisabled(names []string) ([]string, bool) {
	var out []string
	seen, changed := map[string]bool{}, false
	for _, name := range names {
		next := []string{name}
		if name == "exec" {
			next = []string{"Bash", "Monitor", "shell_open", "shell_send"}
			changed = true
		}
		for _, n := range next {
			if !seen[n] {
				out = append(out, n)
				seen[n] = true
			}
		}
	}
	return out, changed
}
