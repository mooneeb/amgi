package event

type EventType string
type EventAction string

const (
	EventTypeIssue             EventType   = "issue"
	EventTypePullRequest       EventType   = "pull_request"
	EventActionOpened          EventAction = "opened"
	EventActionAssigned        EventAction = "assigned"
	EventActionReviewRequested EventAction = "review_requested"
)

var (
	EventTypeIssueActions = []string{
		string(EventActionOpened),
		string(EventActionAssigned),
	}
	EventTypePullRequestActions = []string{
		string(EventActionReviewRequested),
		string(EventActionAssigned),
	}
)

// Event is a normalized view of a GitHub issues or pull_request webhook payload
// (see internal/webhook/parse.go). Fields are used for config resolution, filtering, and persistence.
type Event struct {
	// Type is the GitHub resource kind: "issue" or "pull_request".
	Type string `json:"type"`
	// Owner is the repository owner (first segment of repository full_name).
	Owner string `json:"owner"`
	// Repo is the repository name without owner (second segment of full_name).
	Repo string `json:"repo"`
	// Number is the issue or pull request number on GitHub.
	Number int `json:"number"`
	// Title is the issue or PR title.
	Title string `json:"title"`
	// Body is the issue or PR description (markdown); empty string if null in the payload.
	Body string `json:"body"`
	// State is the GitHub state (e.g. open, closed; PRs may include merged when applicable).
	State string `json:"state"`
	// Action is the normalized webhook action AMGI uses (e.g. opened, assigned, review_requested).
	Action EventAction `json:"action"`
	// Labels is the list of label names on the issue or PR.
	Labels []string `json:"labels"`
	// Assignees is GitHub login names of users assigned to the issue or PR.
	Assignees []string `json:"assignees"`
	// Author is the GitHub login of the user who opened the issue or PR.
	Author string `json:"author"`
	// Branch is the head branch ref for pull requests; empty for issues.
	Branch string `json:"branch"`
	// Reviewers is the GitHub logins of requested reviewers (pull requests only).
	Reviewers []string `json:"reviewers"`
	// URL is the public html_url of the issue or PR on GitHub.
	URL string `json:"url"`
}

type RetryEvent struct {
	Event      *Event
	RetryCount int
}
