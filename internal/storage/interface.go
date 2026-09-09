package storage

import (
	"time"

	"github.com/neul-labs/m9m/internal/model"
	"github.com/neul-labs/m9m/internal/tenancy"
)

// WorkflowStorage defines the interface for workflow persistence
type WorkflowStorage interface {
	// Workspace operations (server-side multi-tenancy).
	// Every storage backend bootstraps tenancy.DefaultID on open so single-
	// tenant self-hosted users have a workspace without any setup. Future
	// milestones add workspace_id columns to the other tables and scope
	// every query.
	SaveWorkspace(workspace *tenancy.Workspace) error
	GetWorkspace(id string) (*tenancy.Workspace, error)
	ListWorkspaces() ([]*tenancy.Workspace, error)
	DeleteWorkspace(id string) error

	// Workflow operations
	SaveWorkflow(workflow *model.Workflow) error
	GetWorkflow(id string) (*model.Workflow, error)
	ListWorkflows(filters WorkflowFilters) ([]*model.Workflow, int, error)
	UpdateWorkflow(id string, workflow *model.Workflow) error
	DeleteWorkflow(id string) error
	ActivateWorkflow(id string) error
	DeactivateWorkflow(id string) error

	// Execution operations
	SaveExecution(execution *model.WorkflowExecution) error
	GetExecution(id string) (*model.WorkflowExecution, error)
	ListExecutions(filters ExecutionFilters) ([]*model.WorkflowExecution, int, error)
	DeleteExecution(id string) error

	// Credential operations
	SaveCredential(credential *Credential) error
	GetCredential(id string) (*Credential, error)
	ListCredentials() ([]*Credential, error)
	UpdateCredential(id string, credential *Credential) error
	DeleteCredential(id string) error

	// Tag operations
	SaveTag(tag *Tag) error
	GetTag(id string) (*Tag, error)
	ListTags() ([]*Tag, error)
	UpdateTag(id string, tag *Tag) error
	DeleteTag(id string) error

	// Raw key-value operations (for webhooks and extensibility)
	SaveRaw(key string, value []byte) error
	GetRaw(key string) ([]byte, error)
	ListKeys(prefix string) ([]string, error)
	DeleteRaw(key string) error

	// Close the storage connection
	Close() error
}

// WorkflowFilters defines filters for listing workflows
type WorkflowFilters struct {
	// WorkspaceID, when set, restricts results to workflows owned by that
	// workspace. Empty means "no workspace filter" — used by single-tenant
	// callers and admin tooling that intentionally crosses tenants.
	WorkspaceID string
	Active      *bool
	Search      string
	Tags        []string
	Offset      int
	Limit       int
}

// ExecutionFilters defines filters for listing executions
type ExecutionFilters struct {
	// WorkspaceID, when set, restricts results to executions belonging to
	// that workspace. Empty means "no workspace filter".
	WorkspaceID string
	WorkflowID  string
	Status      string
	Offset      int
	Limit       int
}

// Credential represents a stored credential.
//
// IsGlobal is the project-scope flag n8n uses to share a credential
// across all projects. m9m does not yet have a projects model — when it
// lands, project_id will be added and IsGlobal will control sharing.
type Credential struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Type      string                 `json:"type"`
	Data      map[string]interface{} `json:"data"`
	IsGlobal  bool                   `json:"isGlobal,omitempty"`
	CreatedAt time.Time              `json:"createdAt"`
	UpdatedAt time.Time              `json:"updatedAt"`
}

// Tag represents a workflow tag
type Tag struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Color     string    `json:"color,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
