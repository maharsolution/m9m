package vcs

import (
	"github.com/neul-labs/m9m/internal/nodes/base"
	"github.com/neul-labs/m9m/internal/nodes/nodeprops"
)

// githubProperties describes the GitHub node.
//
// vcs.GitHubNode reads `resource` (repository | issue | pullRequest |
// user), `operation` (get | list | getAuthenticated), `owner`,
// `repository`, `issueNumber`, `pullNumber` and `username`. Every
// resource uses a different subset of those fields, so each one is
// shown for the resources that consume it.
func githubProperties() []base.NodeProperty {
	resources := []base.Option{
		{Name: "Repository", Value: "repository"},
		{Name: "Issue", Value: "issue"},
		{Name: "Pull request", Value: "pullRequest"},
		{Name: "User", Value: "user"},
	}
	ops := []base.Option{
		{Name: "Get", Value: "get"},
		{Name: "List", Value: "list"},
		{Name: "Get authenticated user", Value: "getAuthenticated"},
	}
	props := []base.NodeProperty{
		nodeprops.Opt("Resource", "resource", "repository",
			"GitHub resource to operate on.",
			resources, true),
		nodeprops.Opt("Operation", "operation", "get",
			"Operation to run on the selected resource.",
			ops, true),
		nodeprops.Str("Owner", "owner", "",
			"Repository owner (user or organisation).",
			"octocat", false).
			WithDisplayOptions(nodeprops.Hide("resource", "user")),
		nodeprops.Str("Repository", "repository", "",
			"Repository name.",
			"Hello-World", false).
			WithDisplayOptions(nodeprops.Show("resource", "repository", "issue", "pullRequest")),
		nodeprops.Num("Issue number", "issueNumber", 0,
			"Number of the issue.", false).
			WithDisplayOptions(nodeprops.Show("resource", "issue")),
		nodeprops.Num("Pull request number", "pullNumber", 0,
			"Number of the pull request.", false).
			WithDisplayOptions(nodeprops.Show("resource", "pullRequest")),
		nodeprops.Str("Username", "username", "",
			"Login of the user to read.",
			"octocat", false).
			WithDisplayOptions(nodeprops.Show("resource", "user", "operation", "get")),
	}
	return append(props, nodeprops.Settings()...)
}

// gitlabProperties describes the GitLab node.
//
// vcs.GitLabNode reads `resource` (issue | mergeRequest | project |
// pipeline | user | branch), `operation`, `baseUrl`, `projectId`,
// `issueIid`, `mergeRequestIid`, `pipelineId`, `userId`, `branch`,
// `title`, `description`, `labels`, `state`, `state_event`,
// `source_branch`, `target_branch`, `merge_commit_message`, `ref`
// and `status`. The resource-specific identifiers are shown for the
// resources that consume them.
func gitlabProperties() []base.NodeProperty {
	resources := []base.Option{
		{Name: "Issue", Value: "issue"},
		{Name: "Merge request", Value: "mergeRequest"},
		{Name: "Project", Value: "project"},
		{Name: "Pipeline", Value: "pipeline"},
		{Name: "User", Value: "user"},
		{Name: "Branch", Value: "branch"},
	}
	ops := []base.Option{
		{Name: "Get", Value: "get"},
		{Name: "Get all", Value: "getAll"},
		{Name: "Create", Value: "create"},
		{Name: "Update", Value: "update"},
		{Name: "Delete", Value: "delete"},
		{Name: "List", Value: "list"},
		{Name: "Merge", Value: "merge"},
		{Name: "Trigger", Value: "trigger"},
		{Name: "Cancel", Value: "cancel"},
		{Name: "Retry", Value: "retry"},
		{Name: "Get authenticated user", Value: "getAuthenticated"},
	}
	props := []base.NodeProperty{
		nodeprops.Opt("Resource", "resource", "issue",
			"GitLab resource to operate on.",
			resources, true),
		nodeprops.Opt("Operation", "operation", "get",
			"Operation to run on the selected resource.",
			ops, true),
		nodeprops.Str("Base URL", "baseUrl", "https://gitlab.com/api/v4",
			"GitLab API root.",
			"https://gitlab.com/api/v4", false),
		nodeprops.Str("Project ID", "projectId", "",
			"Project the resource belongs to.",
			"12345", false).
			WithDisplayOptions(nodeprops.Show("resource", "issue", "mergeRequest", "project", "pipeline", "branch")),
		nodeprops.Num("Issue IID", "issueIid", 0,
			"Internal ID of the issue.", false).
			WithDisplayOptions(nodeprops.Show("resource", "issue")),
		nodeprops.Num("Merge request IID", "mergeRequestIid", 0,
			"Internal ID of the merge request.", false).
			WithDisplayOptions(nodeprops.Show("resource", "mergeRequest")),
		nodeprops.Num("Pipeline ID", "pipelineId", 0,
			"ID of the pipeline.", false).
			WithDisplayOptions(nodeprops.Show("resource", "pipeline")),
		nodeprops.Num("User ID", "userId", 0,
			"ID of the user.", false).
			WithDisplayOptions(nodeprops.Show("resource", "user")),
		nodeprops.Str("Branch", "branch", "",
			"Branch name.",
			"main", false).
			WithDisplayOptions(nodeprops.Show("resource", "branch")),
		nodeprops.Str("Title", "title", "",
			"Title of the issue or merge request.",
			"Fix the importer", false).
			WithDisplayOptions(nodeprops.Show("resource", "issue", "mergeRequest")),
		nodeprops.Required(nodeprops.Text("Description", "description", "",
			"Body of the issue or merge request (Markdown).",
			"", 6)).
			WithDisplayOptions(nodeprops.Show("resource", "issue", "mergeRequest")),
		nodeprops.Str("Labels", "labels", "",
			"Comma-separated labels.",
			"bug,urgent", false).
			WithDisplayOptions(nodeprops.Show("resource", "issue", "mergeRequest")),
		nodeprops.Str("State", "state", "",
			"State of the issue or merge request (opened / closed).",
			"opened", false).
			WithDisplayOptions(nodeprops.Show("resource", "issue", "mergeRequest")),
		nodeprops.Str("State event", "state_event", "",
			"Event applied to the issue (close / reopen).",
			"close", false).
			WithDisplayOptions(nodeprops.Show("resource", "issue")),
		nodeprops.Str("Source branch", "source_branch", "",
			"Source branch of the merge request.",
			"feature/new-thing", false).
			WithDisplayOptions(nodeprops.Show("resource", "mergeRequest")),
		nodeprops.Str("Target branch", "target_branch", "",
			"Target branch of the merge request.",
			"main", false).
			WithDisplayOptions(nodeprops.Show("resource", "mergeRequest")),
		nodeprops.Str("Merge commit message", "merge_commit_message", "",
			"Commit message used when the merge request is merged.",
			"Merge request !1", false).
			WithDisplayOptions(nodeprops.Show("resource", "mergeRequest")),
		nodeprops.Str("Ref", "ref", "",
			"Git ref the pipeline runs against.",
			"main", false).
			WithDisplayOptions(nodeprops.Show("resource", "pipeline")),
		nodeprops.Str("Status", "status", "",
			"Pipeline status used as a filter.",
			"success", false).
			WithDisplayOptions(nodeprops.Show("resource", "pipeline")),
	}
	return append(props, nodeprops.Settings()...)
}
