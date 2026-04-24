package resolve

import (
	"fmt"
	"time"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/event"
	pollingconstants "github.com/mooneeb/amgi/internal/github/polling/constants"
	processorconstants "github.com/mooneeb/amgi/internal/processor/constants"
)

// ResolveOwner returns the first Owner stanza matching ownerName.
// Use ResolveOwnerRepo when both the owner and repo are known.
func ResolveOwner(
	config *config.Config,
	ownerName string,
) (*config.Owner, error) {
	for _, owner := range config.GitHub.Owners {
		if owner.Name == ownerName {
			return &owner, nil
		}
	}
	return nil, fmt.Errorf("owner %s not found in config", ownerName)
}

func ResolveRepository(
	owner *config.Owner,
	repoName string,
) (*config.Repository, error) {
	for _, repo := range owner.Repositories {
		if repo.Name == repoName {
			return &repo, nil
		}
	}
	return nil, fmt.Errorf("repository %s not found under owner %s", repoName, owner.Name)
}

// ResolveOwnerRepo returns the (Owner, Repository) pair matching ownerName
// and repoName. The config may contain multiple Owner stanzas sharing the
// same Name — for example, when a single GitHub owner runs different Modes
// across different repos — in which case the stanza whose Repositories list
// contains repoName is returned.
//
// Returns an error if no matching pair exists.
func ResolveOwnerRepo(
	cfg *config.Config,
	ownerName, repoName string,
) (*config.Owner, *config.Repository, error) {
	for _, owner := range cfg.GitHub.Owners {
		if owner.Name != ownerName {
			continue
		}
		for _, repo := range owner.Repositories {
			if repo.Name == repoName {
				return &owner, &repo, nil
			}
		}
	}
	return nil, nil, fmt.Errorf("no owner %q with repo %q found in config", ownerName, repoName)
}

func ResolveActions(
	owner *config.Owner,
	repo *config.Repository,
	et event.EventType,
) ([]string, error) {
	if repo.Actions != nil {
		switch et {
		case event.EventTypeIssue:
			if repo.Actions.Issues != nil {
				return repo.Actions.Issues, nil
			} else {
				return event.EventTypeIssueActions, nil
			}
		case event.EventTypePullRequest:
			if repo.Actions.PullRequests != nil {
				return repo.Actions.PullRequests, nil
			} else {
				return event.EventTypePullRequestActions, nil
			}
		}
	} else if owner.Actions != nil {
		switch et {
		case event.EventTypeIssue:
			if owner.Actions.Issues != nil {
				return owner.Actions.Issues, nil
			} else {
				return event.EventTypeIssueActions, nil
			}
		case event.EventTypePullRequest:
			if owner.Actions.PullRequests != nil {
				return owner.Actions.PullRequests, nil
			} else {
				return event.EventTypePullRequestActions, nil
			}
		}
	} else {
		switch et {
		case event.EventTypeIssue:
			return event.EventTypeIssueActions, nil
		case event.EventTypePullRequest:
			return event.EventTypePullRequestActions, nil
		}
	}
	return nil, fmt.Errorf("no actions found for owner %s or repo %s for event type %s", owner.Name, repo.Name, et)
}

// ResolveFilters walks the repo → owner → global hierarchy and returns the
// first non-nil *config.Filters it finds, or nil if no level has filters. A
// nil return means "no filters configured; match all events" per the
// architecture (see docs/architecture.md, section "Filter engine").
func ResolveFilters(
	config *config.Config,
	owner *config.Owner,
	repo *config.Repository,
) *config.Filters {
	if repo.Filters != nil {
		return repo.Filters
	} else if owner.Filters != nil {
		return owner.Filters
	} else if config.Filters != nil {
		return config.Filters
	}
	return nil
}

func ResolveMarvinConfig(
	config *config.Config,
	owner *config.Owner,
	repo *config.Repository,
) (*config.MarvinConfig, error) {
	if repo.MarvinConfigID != "" {
		return resolveMarvinConfigId(config, repo.MarvinConfigID)
	} else if owner.MarvinConfigID != "" {
		return resolveMarvinConfigId(config, owner.MarvinConfigID)
	} else {
		return nil, fmt.Errorf("no marvin config id found for owner %s or repo %s", owner.Name, repo.Name)
	}
}

func resolveMarvinConfigId(
	config *config.Config,
	id string,
) (*config.MarvinConfig, error) {
	for _, c := range config.Marvin.Configs {
		if c.ID == id {
			return &c, nil
		}
	}
	return nil, fmt.Errorf("marvin config id %s not found", id)
}

func ResolvePollingInterval(
	owner *config.Owner,
) time.Duration {
	if owner.PollingIntervalSeconds != nil {
		return time.Duration(*owner.PollingIntervalSeconds) * time.Second
	}
	return pollingconstants.DefaultPollingInterval
}

func ResolveRetryInterval(
	cfg *config.Config,
) time.Duration {
	if cfg.RetryIntervalSeconds != nil {
		return time.Duration(*cfg.RetryIntervalSeconds) * time.Second
	}
	return processorconstants.DefaultRetryInterval
}
