package webhook

import (
	"testing"

	"github.com/mooneeb/amgi/internal/event"
	"github.com/mooneeb/amgi/internal/logger"
)

func TestNormalizeGithubIssuePayload(t *testing.T) {
	tests := []struct {
		name      string
		payload   []byte
		eventType event.EventType
		owner     string
		repo      string
		wantErr   bool
	}{
		{
			name:      "valid issue payload & correct event type",
			payload:   []byte(`{"action":"opened","issue":{"number":1,"title":"Fix login bug","body":"The login page crashes","state":"open","labels":[{"name":"bug"}],"assignees":[{"login":"moon"}],"html_url":"https://github.com/acme/foo/issues/42","user":{"login":"zain"}},"repository":{"full_name":"acme/foo"}}`),
			owner:     "test",
			repo:      "test",
			eventType: event.EventTypeIssue,
			wantErr:   false,
		},
		{
			name:      "null body in issue payload",
			payload:   []byte(`{"action":"opened","issue":{"number":2,"title":"Fix login bug","body":null,"state":"open","labels":[{"name":"bug"}],"assignees":[{"login":"moon"}],"html_url":"https://github.com/acme/foo/issues/42","user":{"login":"zain"}},"repository":{"full_name":"acme/foo"}}`),
			owner:     "test",
			repo:      "test",
			eventType: event.EventTypeIssue,
			wantErr:   false,
		},
		{
			name:      "malformed json payload",
			payload:   []byte(`invalid payload`),
			owner:     "test",
			repo:      "test",
			eventType: event.EventTypeIssue,
			wantErr:   true,
		},
		{
			name:      "invalid event type",
			payload:   []byte(`{"action":"opened","issue":{"number":3,"title":"Fix login bug","body":"The login page crashes","state":"open","labels":[{"name":"bug"}],"assignees":[{"login":"moon"}],"html_url":"https://github.com/acme/foo/issues/42","user":{"login":"zain"}},"repository":{"full_name":"acme/foo"}}`),
			owner:     "test",
			repo:      "test",
			eventType: event.EventType("deployment"),
			wantErr:   true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NormalizeGithubWebhookPayload(test.payload, test.eventType, logger.New())
			if (err != nil) != test.wantErr {
				t.Errorf("NormalizeGithubIssuePayload(%s, %s) = %v, want %v", test.payload, test.eventType, err, test.wantErr)
			}
		})
	}
}
