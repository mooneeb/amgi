package resolve

import (
	"fmt"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/event"
)

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

func ResolveFilters(
	config *config.Config,
	owner *config.Owner,
	repo *config.Repository,
) (*config.Filters, error) {
	if repo.Filters != nil {
		return repo.Filters, nil
	} else if owner.Filters != nil {
		return owner.Filters, nil
	} else if config.Filters != nil {
		return config.Filters, nil
	} else {
		return nil, nil
	}
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
