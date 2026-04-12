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

type Event struct {
	Type      string      `json:"type"`
	Org       string      `json:"org"`
	Repo      string      `json:"repo"`
	Number    int         `json:"number"`
	Title     string      `json:"title"`
	Body      string      `json:"body"`
	State     string      `json:"state"`
	Action    EventAction `json:"action"`
	Labels    []string    `json:"labels"`
	Assignees []string    `json:"assignees"`
	Author    string      `json:"author"`
	Branch    string      `json:"branch"`
	Reviewers []string    `json:"reviewers"`
	URL       string      `json:"url"`
}
