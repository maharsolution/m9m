package cli

import (
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// executeCommandProperties describes the Execute Command node.
//
// Every field below is read by cli.ExecuteNode.Execute — the previous
// version advertised `cwd` and a string `shell` while the executor
// reads `workDir` and a boolean `shell`, so the panel wrote values
// the node never looked at.
//
// The sandbox knobs (isolation level, network access, memory ceiling,
// unsafe fallback) only apply while `sandboxEnabled` is true, hence
// the conditional visibility rules.
func executeCommandProperties() []base.NodeProperty {
	props := []base.NodeProperty{
		nodeprops.Str("Command", "command", "",
			"Command to execute. Arguments are passed separately unless shell mode is enabled.",
			"echo 'Hello from m9m!'", true),
		nodeprops.JSON("Arguments", "args", "[]",
			"Command arguments as a JSON array of strings."),
		nodeprops.Str("Working directory", "workDir", "",
			"Directory the command runs in (empty = the engine's working directory).",
			"/data", false),
		nodeprops.Bool("Run through a shell", "shell", false,
			"When enabled the command is handed to a shell, so pipes and && are allowed."),
		nodeprops.Bool("Sandbox the command", "sandboxEnabled", true,
			"Run the command inside the m9m sandbox instead of directly on the host."),
		nodeprops.Bool("Allow unsafe fallback", "allowUnsafeFallback", false,
			"Fall back to running the command unsandboxed when the sandbox cannot be started."),
		nodeprops.Opt("Isolation level", "isolationLevel", "standard",
			"How much of the host a sandboxed command may see.",
			[]base.Option{
				{Name: "None", Value: "none"},
				{Name: "Minimal", Value: "minimal"},
				{Name: "Standard", Value: "standard"},
				{Name: "Strict", Value: "strict"},
				{Name: "Paranoid", Value: "paranoid"},
			}, false).
			WithDisplayOptions(nodeprops.ShowBool("sandboxEnabled", true)),
		nodeprops.Opt("Network access", "networkAccess", "host",
			"Network access granted to the sandboxed command.",
			[]base.Option{
				{Name: "Host network", Value: "host"},
				{Name: "Isolated", Value: "isolated"},
				{Name: "Loopback only", Value: "loopback"},
			}, false).
			WithDisplayOptions(nodeprops.ShowBool("sandboxEnabled", true)),
		nodeprops.Num("Timeout (seconds)", "timeout", 60,
			"Hard timeout for the command in seconds.", false),
		nodeprops.Num("Max memory (MB)", "maxMemoryMB", 512,
			"Memory ceiling for the sandboxed command in megabytes.", false).
			WithDisplayOptions(nodeprops.ShowBool("sandboxEnabled", true)),
		nodeprops.Bool("Pipe input from previous node", "inputFromPrevious", false,
			"Send the previous node's JSON output to the command on stdin."),
		nodeprops.Opt("Output format", "outputFormat", "text",
			"How the command output is returned on the output item.",
			[]base.Option{
				{Name: "Text", Value: "text"},
				{Name: "JSON", Value: "json"},
				{Name: "Lines", Value: "lines"},
			}, false),
		nodeprops.JSON("Environment variables", "env", "{}",
			"Environment variables handed to the command (JSON object of name → value)."),
		nodeprops.JSON("Additional mounts", "additionalMounts", "[]",
			"Extra host paths mounted into the sandbox (JSON array of {source, target} objects)."),
	}
	return append(props, nodeprops.Settings()...)
}
