package resolve

import (
	"fmt"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/event"
)

func ResolveOrganization(
	config *config.Config,
	orgName string,
) (*config.Organization, error) {
	for _, org := range config.GitHub.Organizations {
		if org.Name == orgName {
			return &org, nil
		}
	}
	return nil, fmt.Errorf("organization %s not found in config", orgName)
}

	func ResolveRepository(
	org *config.Organization,
	repoName string,
) (*config.Repository, error) {
	for _, repo := range org.Repositories {
		if repo.Name == repoName {
			return &repo, nil
		}
	}
	return nil, fmt.Errorf("repository %s not found in organization %s", repoName, org.Name)
}

func ResolveActions(
	org *config.Organization,
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
	} else if org.Actions != nil {
		switch et {
		case event.EventTypeIssue:
			if org.Actions.Issues != nil {
				return org.Actions.Issues, nil
			} else {
				return event.EventTypeIssueActions, nil
			}
		case event.EventTypePullRequest:
			if org.Actions.PullRequests != nil {
				return org.Actions.PullRequests, nil
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
	return nil, fmt.Errorf("no actions found for org %s or repo %s for event type %s", org.Name, repo.Name, et)
}

func ResolveFilters(
	config *config.Config,
	org *config.Organization,
	repo *config.Repository,
) (*config.Filters, error) {
	if repo.Filters != nil {
		return repo.Filters, nil
	} else if org.Filters != nil {
		return org.Filters, nil
	} else if config.Filters != nil {
		return config.Filters, nil
	} else {
		return nil, nil
	}
}

func ResolveMarvinConfig(
	config *config.Config,
	org *config.Organization,
	repo *config.Repository,
) (*config.MarvinConfig, error) {
	if repo.MarvinConfigID != "" {
		return resolveMarvinConfigId(config, repo.MarvinConfigID)
	} else if org.MarvinConfigID != "" {
		return resolveMarvinConfigId(config, org.MarvinConfigID)
	} else {
		return nil, fmt.Errorf("no marvin config id found for org %s or repo %s", org.Name, repo.Name)
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
