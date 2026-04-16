package webhook

import (
	"testing"

	"github.com/mooneeb/amgi/internal/event"
	"github.com/mooneeb/amgi/internal/logger"
)

func TestNormalizeGithubPayload(t *testing.T) {
	tests := []struct {
		name      string
		payload   []byte
		eventType event.EventType
		wantErr   bool
	}{
		{
			name:      "valid issue payload & correct event type",
			payload:   []byte(`{"action":"opened","issue":{"number":42,"title":"Fix login bug","body":"The login page crashes","state":"open","labels":[{"name":"bug"}],"assignees":[{"login":"moon"}],"html_url":"https://github.com/acme/foo/issues/42","user":{"login":"zain"}},"repository":{"full_name":"acme/foo"}}`),
			eventType: event.EventTypeIssue,
			wantErr:   false,
		},
		{
			name:      "Valid PR payload & correct event type",
			payload:   []byte(`{"action":"review_requested","repository":{"full_name":"acme/foo"},"pull_request":{"number":99,"title":"Add OAuth support","body":"Implements OAuth2 flow for third-party apps","state":"open","labels":[{"name":"feature"},{"name":"auth"}],"assignees":[{"login":"moon"},{"login":"zain"}],"html_url":"https://github.com/acme/foo/pull/99","user":{"login":"moon"},"head":{"ref":"feature/oauth"},"requested_reviewers":[{"login":"zain"},{"login":"ali"}]}}`),
			eventType: event.EventTypePullRequest,
			wantErr:   false,
		},
		{
			name:      "null body in issue payload",
			payload:   []byte(`{"action":"opened","issue":{"number":42,"title":"Fix login bug","body":null,"state":"open","labels":[{"name":"bug"}],"assignees":[{"login":"moon"}],"html_url":"https://github.com/acme/foo/issues/42","user":{"login":"zain"}},"repository":{"full_name":"acme/foo"}}`),
			eventType: event.EventTypeIssue,
			wantErr:   false,
		},
		{
			name:      "malformed json payload",
			payload:   []byte(`invalid payload`),
			eventType: event.EventTypeIssue,
			wantErr:   true,
		},
		{
			name:      "invalid repository full name",
			payload:   []byte(`{"action":"opened","issue":{"number":42,"title":"Fix login bug","body":"The login page crashes","state":"open","labels":[{"name":"bug"}],"assignees":[{"login":"moon"}],"html_url":"https://github.com/acme/foo/issues/42","user":{"login":"zain"}},"repository":{"full_name":"acme/foo/bar"}}`),
			eventType: event.EventTypeIssue,
			wantErr:   true,
		},
		{
			name:      "invalid event type",
			payload:   []byte(`{"action":"opened","issue":{"number":42,"title":"Fix login bug","body":"The login page crashes","state":"open","labels":[{"name":"bug"}],"assignees":[{"login":"moon"}],"html_url":"https://github.com/acme/foo/issues/42","user":{"login":"zain"}},"repository":{"full_name":"acme/foo"}}`),
			eventType: event.EventType("invalid"),
			wantErr:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NormalizeGithubPayload(test.payload, test.eventType, logger.New())
			if (err != nil) != test.wantErr {
				t.Errorf("NormalizeGithubPayload(%s, %s) = %v, want %v", test.payload, test.eventType, err, test.wantErr)
			}
		})
	}
}
