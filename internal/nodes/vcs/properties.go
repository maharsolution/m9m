package vcs

import "github.com/neul-labs/m9m/internal/nodes/base"

// BUG-UI-2026-09-09-001 — populate the `Properties` field for
// the VCS nodes. Each helper mirrors n8n's
// `INodeTypeDescription.properties` for the corresponding
// `n8n-nodes-base.<node>` type.

// githubProperties returns the GitHub node's parameter
// descriptors.
func githubProperties() []base.NodeProperty {
	resources := []base.Option{
		{Name: "File", Value: "file"},
		{Name: "Issue", Value: "issue"},
		{Name: "Pull Request", Value: "pullRequest"},
		{Name: "Release", Value: "release"},
		{Name: "Repository", Value: "repository"},
		{Name: "User", Value: "user"},
	}
	ops := []base.Option{
		{Name: "Create", Value: "create"},
		{Name: "Get", Value: "get"},
		{Name: "List", Value: "list"},
		{Name: "Update", Value: "update"},
		{Name: "Delete", Value: "delete"},
		{Name: "Edit", Value: "edit"},
	}
	props := []base.NodeProperty{
		base.StringOpt("Resource", "resource", "issue", "GitHub resource to operate on.", resources, true),
		base.StringOpt("Operation", "operation", "create", "Operation on the resource.", ops, true),
		base.StringProp("Owner", "owner", "", "Repository owner (user or org).", "", true),
		base.StringProp("Repository", "repository", "", "Repository name.", "", true),
		base.StringProp("Title", "title", "", "Issue / PR title.", "", false),
		base.TextProp("Body", "body", "", "Issue / PR body (Markdown).", "", 6),
		base.StringProp("Branch", "branch", "", "Branch name (for file / PR operations).", "main", false),
		base.StringProp("Base Branch", "base", "", "PR base branch.", "main", false),
		base.StringProp("State", "state", "open", "Issue / PR state (open / closed).", "open", false),
		base.JsonProp("Labels", "labels", "[]", "Issue / PR labels (array of strings)."),
		base.JsonProp("Assignees", "assignees", "[]", "Assignees (array of usernames)."),
		base.StringProp("File Path", "filePath", "", "Path within the repo (file resource).", "", false),
		base.StringProp("File Content", "fileContent", "", "Content of the file (file resource).", "", false),
		base.StringProp("Commit Message", "commitMessage", "", "Commit message (file resource).", "", false),
	}
	return append(props, base.CommonSettings()...)
}

// gitlabProperties returns the GitLab node's parameter
// descriptors.
func gitlabProperties() []base.NodeProperty {
	resources := []base.Option{
		{Name: "File", Value: "file"},
		{Name: "Issue", Value: "issue"},
		{Name: "Merge Request", Value: "mergeRequest"},
		{Name: "Pipeline", Value: "pipeline"},
		{Name: "Project", Value: "project"},
	}
	ops := []base.Option{
		{Name: "Create", Value: "create"},
		{Name: "Get", Value: "get"},
		{Name: "List", Value: "list"},
		{Name: "Update", Value: "update"},
		{Name: "Delete", Value: "delete"},
	}
	props := []base.NodeProperty{
		base.StringOpt("Resource", "resource", "issue", "GitLab resource to operate on.", resources, true),
		base.StringOpt("Operation", "operation", "create", "Operation on the resource.", ops, true),
		base.StringProp("Project ID", "projectId", "", "GitLab project ID (numeric).", "", true),
		base.StringProp("Title", "title", "", "Issue / MR title.", "", false),
		base.TextProp("Body", "body", "", "Issue / MR body (Markdown).", "", 6),
		base.StringProp("Branch", "branch", "", "Branch name.", "main", false),
		base.StringProp("Source Branch", "sourceBranch", "", "MR source branch.", "main", false),
		base.StringProp("Target Branch", "targetBranch", "", "MR target branch.", "main", false),
		base.StringProp("State", "state", "opened", "Issue / MR state (opened / closed / merged).", "opened", false),
		base.JsonProp("Labels", "labels", "[]", "Labels (array of strings)."),
	}
	return append(props, base.CommonSettings()...)
}
