package config

const (
	DefaultWebhookPort            int    = 8080
	DefaultWebhookPath            string = "/webhooks/github"
	MarvinTitleAutoCompleteHeader string = "X-Auto-Complete"
	MaxRetryAttempts              int    = 3
)

// Config is the root AMGI configuration document.
type Config struct {
	// Version is the config schema version (e.g. "1").
	Version string `json:"version" yaml:"version" jsonschema:"enum=1"`
	// Filters are global rules for which issues and PRs create Marvin tasks. Repositories may replace these with per-repo filters.
	Filters *Filters `json:"filters,omitempty" yaml:"filters,omitempty"`
	// WebhookServer is where AMGI listens for GitHub webhooks. Required when any owner uses webhook mode.
	WebhookServer *WebhookServer `json:"webhook_server,omitempty" yaml:"webhook_server,omitempty"`
	// GitHub lists owners and repositories to watch.
	GitHub GitHub `json:"github" yaml:"github"`
	// Marvin holds named configs (lists, labels, templates) referenced by marvin_config_id.
	Marvin Marvin `json:"marvin" yaml:"marvin"`
}

// Filters holds global filter rules for issues and pull requests.
type Filters struct {
	// Issues selects which GitHub issues produce tasks. Omitted means no issue filters at this level.
	Issues *IssueFilters `json:"issues,omitempty" yaml:"issues,omitempty"`
	// PullRequests selects which pull requests produce tasks. Omitted means no PR filters at this level.
	PullRequests *PullRequestFilters `json:"pull_requests,omitempty" yaml:"pull_requests,omitempty"`
}

// FieldOperators groups Kubernetes-style operators (in, notIn, exists, doesNotExist).
type FieldOperators struct {
	// In requires the value to match one of these strings (e.g. label name, login). For title, entries are regex patterns.
	In []string `json:"in,omitempty" yaml:"in,omitempty"`
	// NotIn requires the value to match none of these strings (or regex patterns for title).
	NotIn []string `json:"notIn,omitempty" yaml:"notIn,omitempty"`
	// Exists, when true, requires at least one; when false, requires none (semantics depend on the parent field, e.g. labels).
	Exists *bool `json:"exists,omitempty" yaml:"exists,omitempty"`
	// DoesNotExist inverts the exists-style check for the parent field.
	DoesNotExist *bool `json:"doesNotExist,omitempty" yaml:"doesNotExist,omitempty"`
}

// IssueFilters is filter rules for GitHub issues.
type IssueFilters struct {
	// Labels filters by issue label names.
	Labels *FieldOperators `json:"labels,omitempty" yaml:"labels,omitempty"`
	// Assignees filters by GitHub assignee logins.
	Assignees *FieldOperators `json:"assignees,omitempty" yaml:"assignees,omitempty"`
	// Author filters by the issue creator's login.
	Author *FieldOperators `json:"author,omitempty" yaml:"author,omitempty"`
	// Title filters by issue title; in/notIn use regex patterns.
	Title *FieldOperators `json:"title,omitempty" yaml:"title,omitempty"`
}

// PullRequestFilters is filter rules for GitHub pull requests.
type PullRequestFilters struct {
	// Labels filters by PR label names.
	Labels *FieldOperators `json:"labels,omitempty" yaml:"labels,omitempty"`
	// Branches filters by branch name; in/notIn may use names or patterns.
	Branches *FieldOperators `json:"branches,omitempty" yaml:"branches,omitempty"`
	// Reviewers filters by reviewer logins.
	Reviewers *FieldOperators `json:"reviewers,omitempty" yaml:"reviewers,omitempty"`
	// Assignees filters by PR assignee logins.
	Assignees *FieldOperators `json:"assignees,omitempty" yaml:"assignees,omitempty"`
	// Author filters by the PR author's login.
	Author *FieldOperators `json:"author,omitempty" yaml:"author,omitempty"`
	// Title filters by PR title; in/notIn use regex patterns.
	Title *FieldOperators `json:"title,omitempty" yaml:"title,omitempty"`
}

// WebhookServer configures AMGI's HTTP listener for GitHub webhooks.
type WebhookServer struct {
	// Port is the TCP port to listen on (default 8080 when omitted).
	Port *int `json:"port,omitempty" yaml:"port,omitempty" jsonschema:"default=8080,minimum=1,maximum=65535"`
	// Path is the URL path for the webhook endpoint; must match GitHub's payload URL (default /webhooks/github when omitted).
	Path *string `json:"path,omitempty" yaml:"path,omitempty" jsonschema:"default=/webhooks/github,pattern=^/"`
}

// GitHub is GitHub connection and source configuration.
type GitHub struct {
	// Owners is the list of GitHub owners to watch (webhook vs polling is per owner).
	Owners []Owner `json:"owners" yaml:"owners"`
}

// ModeType is how events are received for an owner (webhook or polling).
type ModeType string

const (
	// ModeWebhook receives GitHub events in real time via webhooks.
	ModeWebhook ModeType = "webhook"
	// ModePolling polls GitHub on a fixed interval (see PollingIntervalSeconds).
	ModePolling ModeType = "polling"
)

// Owner is one GitHub owner's watch configuration.
type Owner struct {
	// Name is the GitHub owner login (user or organization).
	Name string `json:"name"`
	// Mode selects webhook or polling for this owner.
	Mode ModeType `json:"mode" yaml:"mode" jsonschema:"enum=webhook,enum=polling"`
	// Actions restricts which webhook actions create tasks (ignored when Mode is polling). Omitted uses defaults.
	Actions *EventActions `json:"actions,omitempty" yaml:"actions,omitempty"`
	// PollingIntervalSeconds is the seconds between poll runs when Mode is polling.
	PollingIntervalSeconds *int `json:"polling_interval_seconds,omitempty" yaml:"polling_interval_seconds,omitempty" jsonschema:"minimum=1"`
	// MarvinConfigID selects the default marvin.configs entry for repos under this owner unless overridden per repository.
	MarvinConfigID string `json:"marvin_config_id" yaml:"marvin_config_id"`
	// Repositories lists repos to watch under this owner.
	Repositories []Repository `json:"repositories" yaml:"repositories"`
	// Filters replaces global filters for this owner only when set.
	Filters *Filters `json:"filters,omitempty" yaml:"filters,omitempty"`
}

// EventActions selects which webhook event actions trigger task creation (ignored for polling).
type EventActions struct {
	// Issues lists issue actions (e.g. opened, assigned) that may create tasks.
	Issues []string `json:"issues,omitempty" yaml:"issues,omitempty" jsonschema:"enum=opened,enum=assigned"`
	// PullRequests lists PR actions (e.g. review_requested, assigned) that may create tasks.
	PullRequests []string `json:"pull_requests,omitempty" yaml:"pull_requests,omitempty" jsonschema:"enum=review_requested,enum=assigned"`
}

// Repository is a repository entry that exists under a GitHub owner.
type Repository struct {
	// Name is the repository name without the owner prefix (not owner/repo).
	Name string `json:"name" yaml:"name"`
	// MarvinConfigID overrides the owner's marvin_config_id for this repo when set.
	MarvinConfigID string `json:"marvin_config_id,omitempty" yaml:"marvin_config_id,omitempty"`
	// Actions overrides the owner's EventActions for this repo (webhook only; ignored for polling).
	Actions *EventActions `json:"actions,omitempty" yaml:"actions,omitempty"`
	// Filters replaces global filters for this repository only when set.
	Filters *Filters `json:"filters,omitempty" yaml:"filters,omitempty"`
}

// Marvin holds Marvin destination and task-creation settings.
type Marvin struct {
	// Configs is the list of named Marvin configs; each ID is referenced by marvin_config_id on GitHub.
	Configs []MarvinConfig `json:"configs" yaml:"configs"`
}

// MarvinConfig is one named Marvin destination referenced by marvin_config_id.
type MarvinConfig struct {
	// ID uniquely identifies this config; github marvin_config_id fields reference it.
	ID string `json:"id" yaml:"id"`
	// ListID is the Marvin parent ID (category, project, or "unassigned" for Inbox). Takes precedence over ListName when both are set.
	ListID string `json:"list_id,omitempty" yaml:"list_id,omitempty"`
	// ListName is a category or project title resolved via the Marvin API (exact match). Ignored when ListID is set.
	ListName string `json:"list_name,omitempty" yaml:"list_name,omitempty"`
	// LabelIDs are Marvin label IDs attached to every task created with this config.
	LabelIDs []string `json:"label_ids,omitempty" yaml:"label_ids,omitempty"`
	// LabelNames are Marvin label titles (exact match), resolved to IDs via the API.
	LabelNames []string `json:"label_names,omitempty" yaml:"label_names,omitempty"`
	// AutoComplete controls Marvin title autocomplete: when true, the title may use Marvin operators and wins over key-value task fields; when false, only templates and explicit task fields apply.
	AutoComplete *bool `json:"auto_complete,omitempty" yaml:"auto_complete,omitempty" jsonschema:"default=true"`
	// Task holds the title and note templates plus optional schedule and Marvin task fields.
	Task MarvinTask `json:"task" yaml:"task"`
}

// MarvinTask is task content and optional Marvin API-backed fields.
type MarvinTask struct {
	// TitleTemplate is a Go text/template for the task title (may include Marvin operators when AutoComplete is true).
	TitleTemplate string `json:"title_template" yaml:"title_template"`
	// NoteTemplate is a Go text/template for the task body (note).
	NoteTemplate string `json:"note_template" yaml:"note_template"`
	// Day is a schedule date (YYYY-MM-DD).
	Day string `json:"day,omitempty" yaml:"day,omitempty" jsonschema:"format=date"`
	// DueDate is the due date (YYYY-MM-DD).
	DueDate string `json:"due_date,omitempty" yaml:"due_date,omitempty" jsonschema:"format=date"`
	// StartDate is the start date (YYYY-MM-DD). With autocomplete, may be sent via title operators rather than the API body.
	StartDate string `json:"start_date,omitempty" yaml:"start_date,omitempty" jsonschema:"format=date"`
	// EndDate is the end date (YYYY-MM-DD). With autocomplete, may be sent via title operators rather than the API body.
	EndDate string `json:"end_date,omitempty" yaml:"end_date,omitempty" jsonschema:"format=date"`
	// PlannedWeek is the Monday of the planned week (YYYY-MM-DD).
	PlannedWeek string `json:"planned_week,omitempty" yaml:"planned_week,omitempty" jsonschema:"format=date"`
	// PlannedMonth is the planned month (YYYY-MM).
	PlannedMonth string `json:"planned_month,omitempty" yaml:"planned_month,omitempty" jsonschema:"pattern=^[0-9]{4}-[0-9]{2}$"`
	// TimeEstimateMs is the duration estimate in milliseconds.
	TimeEstimateMs *int64 `json:"time_estimate_ms,omitempty" yaml:"time_estimate_ms,omitempty" jsonschema:"minimum=0"`
	// Priority is Marvin priority (isStarred): 0 none, 1 yellow, 2 orange, 3 red.
	Priority *int `json:"priority,omitempty" yaml:"priority,omitempty" jsonschema:"minimum=0,maximum=3"`
	// Frog is Marvin frog level (isFrogged): 0 none, 1 normal, 2 baby, 3 monster.
	Frog *int `json:"frog,omitempty" yaml:"frog,omitempty" jsonschema:"minimum=0,maximum=3"`
	// IsReward is false for a normal task that earns kudos, or true for a reward-style task when set.
	IsReward *bool `json:"is_reward,omitempty" yaml:"is_reward,omitempty"`
	// RewardPoints is the reward points to attach to the task.
	RewardPoints *float64 `json:"reward_points,omitempty" yaml:"reward_points,omitempty"`
	// Section is the daily section name (Morning, Afternoon, Evening) or a custom section ID.
	Section string `json:"section,omitempty" yaml:"section,omitempty"`
	// ReviewDate is the review date (YYYY-MM-DD).
	ReviewDate string `json:"review_date,omitempty" yaml:"review_date,omitempty" jsonschema:"format=date"`
}
