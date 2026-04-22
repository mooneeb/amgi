package resolve

import (
	"slices"
	"testing"
	"time"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/event"
	pollingconstants "github.com/mooneeb/amgi/internal/github/polling/constants"
	processorconstants "github.com/mooneeb/amgi/internal/processor/constants"
)

// intPtr returns a pointer to i. Helper for optional-int config fields
// that use *int to distinguish "unset" (nil) from "explicitly zero".
func intPtr(i int) *int { return &i }

// -----------------------------------------------------------------------------
// ResolveOwner
// -----------------------------------------------------------------------------

func TestResolveOwner(t *testing.T) {
	cfg := &config.Config{
		GitHub: config.GitHub{
			Owners: []config.Owner{
				{Name: "acme"},
				{Name: "other"},
			},
		},
	}
	cases := []struct {
		name      string
		ownerName string
		wantName  string
		wantErr   bool
	}{
		{"first owner", "acme", "acme", false},
		{"second owner", "other", "other", false},
		{"not found", "missing", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveOwner(cfg, tc.ownerName)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if got == nil {
				t.Fatalf("got nil owner, want non-nil")
			}
			if got.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tc.wantName)
			}
		})
	}
}

func TestResolveOwner_EmptyConfig(t *testing.T) {
	cfg := &config.Config{}
	_, err := ResolveOwner(cfg, "anything")
	if err == nil {
		t.Errorf("got nil error, want not-found error for empty Owners")
	}
}

// -----------------------------------------------------------------------------
// ResolveRepository
// -----------------------------------------------------------------------------

func TestResolveRepository(t *testing.T) {
	owner := &config.Owner{
		Name: "acme",
		Repositories: []config.Repository{
			{Name: "foo"},
			{Name: "bar"},
		},
	}
	cases := []struct {
		name     string
		repoName string
		wantName string
		wantErr  bool
	}{
		{"first repo", "foo", "foo", false},
		{"second repo", "bar", "bar", false},
		{"not found", "baz", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveRepository(owner, tc.repoName)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if got == nil {
				t.Fatalf("got nil repo, want non-nil")
			}
			if got.Name != tc.wantName {
				t.Errorf("Name = %q, want %q", got.Name, tc.wantName)
			}
		})
	}
}

func TestResolveRepository_EmptyOwner(t *testing.T) {
	owner := &config.Owner{Name: "acme"}
	_, err := ResolveRepository(owner, "anything")
	if err == nil {
		t.Errorf("got nil error, want not-found error for empty Repositories")
	}
}

// -----------------------------------------------------------------------------
// ResolveActions
// -----------------------------------------------------------------------------

func TestResolveActions(t *testing.T) {
	customIssues := []string{"opened"}
	customPRs := []string{"review_requested"}

	cases := []struct {
		name    string
		owner   *config.Owner
		repo    *config.Repository
		et      event.EventType
		want    []string
		wantErr bool
	}{
		{
			name:  "repo overrides with custom issues",
			owner: &config.Owner{Name: "acme"},
			repo: &config.Repository{
				Name:    "foo",
				Actions: &config.EventActions{Issues: customIssues},
			},
			et:   event.EventTypeIssue,
			want: customIssues,
		},
		{
			name:  "repo.Actions set but Issues nil → default issues",
			owner: &config.Owner{Name: "acme"},
			repo: &config.Repository{
				Name:    "foo",
				Actions: &config.EventActions{PullRequests: customPRs},
			},
			et:   event.EventTypeIssue,
			want: event.EventTypeIssueActions,
		},
		{
			name:  "repo overrides with custom PRs",
			owner: &config.Owner{Name: "acme"},
			repo: &config.Repository{
				Name:    "foo",
				Actions: &config.EventActions{PullRequests: customPRs},
			},
			et:   event.EventTypePullRequest,
			want: customPRs,
		},
		{
			name:  "repo.Actions set but PRs nil → default PRs",
			owner: &config.Owner{Name: "acme"},
			repo: &config.Repository{
				Name:    "foo",
				Actions: &config.EventActions{Issues: customIssues},
			},
			et:   event.EventTypePullRequest,
			want: event.EventTypePullRequestActions,
		},
		{
			name: "repo.Actions nil → owner issues win",
			owner: &config.Owner{
				Name:    "acme",
				Actions: &config.EventActions{Issues: customIssues},
			},
			repo: &config.Repository{Name: "foo"},
			et:   event.EventTypeIssue,
			want: customIssues,
		},
		{
			name: "repo.Actions nil, owner Actions set but Issues nil → default",
			owner: &config.Owner{
				Name:    "acme",
				Actions: &config.EventActions{PullRequests: customPRs},
			},
			repo: &config.Repository{Name: "foo"},
			et:   event.EventTypeIssue,
			want: event.EventTypeIssueActions,
		},
		{
			name: "repo.Actions nil → owner PRs win",
			owner: &config.Owner{
				Name:    "acme",
				Actions: &config.EventActions{PullRequests: customPRs},
			},
			repo: &config.Repository{Name: "foo"},
			et:   event.EventTypePullRequest,
			want: customPRs,
		},
		{
			name: "repo.Actions nil, owner Actions set but PRs nil → default",
			owner: &config.Owner{
				Name:    "acme",
				Actions: &config.EventActions{Issues: customIssues},
			},
			repo: &config.Repository{Name: "foo"},
			et:   event.EventTypePullRequest,
			want: event.EventTypePullRequestActions,
		},
		{
			name:  "both nil → default issues",
			owner: &config.Owner{Name: "acme"},
			repo:  &config.Repository{Name: "foo"},
			et:    event.EventTypeIssue,
			want:  event.EventTypeIssueActions,
		},
		{
			name:  "both nil → default PRs",
			owner: &config.Owner{Name: "acme"},
			repo:  &config.Repository{Name: "foo"},
			et:    event.EventTypePullRequest,
			want:  event.EventTypePullRequestActions,
		},
		{
			name:    "unknown event type → error",
			owner:   &config.Owner{Name: "acme"},
			repo:    &config.Repository{Name: "foo"},
			et:      event.EventType("unknown"),
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveActions(tc.owner, tc.repo, tc.et)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// ResolveFilters
// -----------------------------------------------------------------------------

func TestResolveFilters(t *testing.T) {
	// Distinct non-nil Filters pointers let us identify which hierarchy level
	// won via pointer equality (not structural equality).
	globalFilters := &config.Filters{Issues: &config.IssueFilters{}}
	ownerFilters := &config.Filters{Issues: &config.IssueFilters{}}
	repoFilters := &config.Filters{Issues: &config.IssueFilters{}}

	cases := []struct {
		name  string
		cfg   *config.Config
		owner *config.Owner
		repo  *config.Repository
		want  *config.Filters
	}{
		{
			name:  "repo filters win",
			cfg:   &config.Config{Filters: globalFilters},
			owner: &config.Owner{Name: "acme", Filters: ownerFilters},
			repo:  &config.Repository{Name: "foo", Filters: repoFilters},
			want:  repoFilters,
		},
		{
			name:  "owner filters win when repo has none",
			cfg:   &config.Config{Filters: globalFilters},
			owner: &config.Owner{Name: "acme", Filters: ownerFilters},
			repo:  &config.Repository{Name: "foo"},
			want:  ownerFilters,
		},
		{
			name:  "global filters fall back when neither repo nor owner",
			cfg:   &config.Config{Filters: globalFilters},
			owner: &config.Owner{Name: "acme"},
			repo:  &config.Repository{Name: "foo"},
			want:  globalFilters,
		},
		{
			name:  "nil when no filters anywhere",
			cfg:   &config.Config{},
			owner: &config.Owner{Name: "acme"},
			repo:  &config.Repository{Name: "foo"},
			want:  nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveFilters(tc.cfg, tc.owner, tc.repo)
			if got != tc.want {
				t.Errorf("got %p (%+v), want %p (%+v)", got, got, tc.want, tc.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// ResolveMarvinConfig
// -----------------------------------------------------------------------------

func TestResolveMarvinConfig(t *testing.T) {
	cfg := &config.Config{
		Marvin: config.Marvin{
			Configs: []config.MarvinConfig{
				{ID: "default", Task: config.MarvinTask{TitleTemplate: "default"}},
				{ID: "owner-cfg", Task: config.MarvinTask{TitleTemplate: "owner"}},
				{ID: "repo-cfg", Task: config.MarvinTask{TitleTemplate: "repo"}},
			},
		},
	}

	cases := []struct {
		name    string
		owner   *config.Owner
		repo    *config.Repository
		wantID  string
		wantErr bool
	}{
		{
			name:   "repo override wins over owner",
			owner:  &config.Owner{Name: "acme", MarvinConfigID: "owner-cfg"},
			repo:   &config.Repository{Name: "foo", MarvinConfigID: "repo-cfg"},
			wantID: "repo-cfg",
		},
		{
			name:   "owner config used when repo has no override",
			owner:  &config.Owner{Name: "acme", MarvinConfigID: "owner-cfg"},
			repo:   &config.Repository{Name: "foo"},
			wantID: "owner-cfg",
		},
		{
			name:    "repo ID not found in configs → error",
			owner:   &config.Owner{Name: "acme", MarvinConfigID: "owner-cfg"},
			repo:    &config.Repository{Name: "foo", MarvinConfigID: "missing"},
			wantErr: true,
		},
		{
			name:    "owner ID not found in configs → error",
			owner:   &config.Owner{Name: "acme", MarvinConfigID: "also-missing"},
			repo:    &config.Repository{Name: "foo"},
			wantErr: true,
		},
		{
			name:    "neither repo nor owner has ID → error",
			owner:   &config.Owner{Name: "acme"},
			repo:    &config.Repository{Name: "foo"},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolveMarvinConfig(cfg, tc.owner, tc.repo)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if got == nil {
				t.Fatalf("got nil MarvinConfig, want non-nil")
			}
			if got.ID != tc.wantID {
				t.Errorf("ID = %q, want %q", got.ID, tc.wantID)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// ResolvePollingInterval
// -----------------------------------------------------------------------------

func TestResolvePollingInterval(t *testing.T) {
	cases := []struct {
		name  string
		owner *config.Owner
		want  time.Duration
	}{
		{
			name:  "override",
			owner: &config.Owner{Name: "acme", PollingIntervalSeconds: intPtr(600)},
			want:  600 * time.Second,
		},
		{
			name:  "override minimum (60s)",
			owner: &config.Owner{Name: "acme", PollingIntervalSeconds: intPtr(60)},
			want:  60 * time.Second,
		},
		{
			name:  "default when nil",
			owner: &config.Owner{Name: "acme"},
			want:  pollingconstants.DefaultPollingInterval,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolvePollingInterval(tc.owner)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// ResolveRetryInterval
// -----------------------------------------------------------------------------

func TestResolveRetryInterval(t *testing.T) {
	cases := []struct {
		name string
		cfg  *config.Config
		want time.Duration
	}{
		{
			name: "override",
			cfg:  &config.Config{RetryIntervalSeconds: intPtr(120)},
			want: 120 * time.Second,
		},
		{
			name: "override minimum (60s)",
			cfg:  &config.Config{RetryIntervalSeconds: intPtr(60)},
			want: 60 * time.Second,
		},
		{
			name: "default when nil",
			cfg:  &config.Config{},
			want: processorconstants.DefaultRetryInterval,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveRetryInterval(tc.cfg)
			if got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
