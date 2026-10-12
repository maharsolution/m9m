package cli

import "github.com/neul-labs/m9m/internal/nodes/base"

// BUG-UI-2026-09-09-001 — populate the `Properties` field for
// the CLI Execute node.

func executeCommandProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		base.StringProp("Command", "command", "", "Shell command to execute. Empty = no-op.", "echo 'Hello from m9m!'", true),
		base.JsonProp("Args", "args", "[]", "Command arguments as a JSON array."),
		base.StringOpt("Shell", "shell", "/bin/sh", "Shell to invoke for the command.", []base.Option{
			{Name: "/bin/sh", Value: "/bin/sh"},
			{Name: "/bin/bash", Value: "/bin/bash"},
			{Name: "cmd.exe (Windows)", Value: "cmd.exe"},
			{Name: "powershell.exe", Value: "powershell.exe"},
		}, false),
		base.StringProp("Working Directory", "cwd", "", "Working directory (empty = current).", "", false),
		base.NumberProp("Timeout (s)", "timeout", 60, "Hard timeout in seconds (1-3600).", false),
		base.BoolProp("Sandbox", "sandbox", false, "Run the command in a sandboxed environment."),
	}
	return append(props, base.CommonSettings()...)
}
