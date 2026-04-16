package polling

import (
	"encoding/json"
	"fmt"

	"github.com/mooneeb/amgi/internal/event"
	igithub "github.com/mooneeb/amgi/internal/github"
)

func NormalizeGithubPollingPayload(
	payload []byte,
	eventType event.EventType,
	owner, repo string,
) (*event.Event, error) {
	switch eventType {
	case event.EventTypeIssue:
		var ppIssue igithub.Issue
		err := json.Unmarshal(payload, &ppIssue)
		if err != nil {
			return nil, err
		}
		return igithub.NormalizeGithubIssuePayload(ppIssue, owner, repo, event.EventActionOpened)
	case event.EventTypePullRequest:
		var ppPullRequest igithub.PullRequest
		err := json.Unmarshal(payload, &ppPullRequest)
		if err != nil {
			return nil, err
		}
		return igithub.NormalizeGithubPullRequestPayload(ppPullRequest, owner, repo, event.EventActionOpened)
	default:
		return nil, fmt.Errorf("invalid event type: %s", string(eventType))
	}
}
