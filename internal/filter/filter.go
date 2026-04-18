package filter

import (
	"fmt"
	"regexp"
	"slices"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/event"
)

func IsIssueMatch(
	event *event.Event,
	filters *config.IssueFilters,
) (bool, error) {

	if filters == nil {
		return true, nil
	}

	m := isFieldsMatch(event.Labels, filters.Labels)
	if !m {
		return false, nil
	}

	m = isFieldsMatch(event.Assignees, filters.Assignees)
	if !m {
		return false, nil
	}

	m, err := isFieldMatch(event.Author, filters.Author)
	if err != nil {
		return false, fmt.Errorf("failed to match author: %w", err)
	}

	if !m {
		return false, nil
	}

	m, err = isFieldMatch(event.Title, filters.Title)
	if err != nil {
		return false, fmt.Errorf("failed to match title: %w", err)
	}

	if !m {
		return false, nil
	}

	return true, nil
}

func IsPullRequestMatch(
	event *event.Event,
	filters *config.PullRequestFilters,
) (bool, error) {

	if filters == nil {
		return true, nil
	}

	m := isFieldsMatch(event.Labels, filters.Labels)
	if !m {
		return false, nil
	}

	m = isFieldsMatch(event.Assignees, filters.Assignees)
	if !m {
		return false, nil
	}

	m = isFieldsMatch(event.Reviewers, filters.Reviewers)
	if !m {
		return false, nil
	}

	m, err := isFieldMatch(event.Author, filters.Author)
	if err != nil {
		return false, fmt.Errorf("failed to match author: %w", err)
	}

	if !m {
		return false, nil
	}

	m, err = isFieldMatch(event.Title, filters.Title)
	if err != nil {
		return false, fmt.Errorf("failed to match title: %w", err)
	}

	if !m {
		return false, nil
	}

	m, err = isFieldMatch(event.Branch, filters.Branches)
	if err != nil {
		return false, fmt.Errorf("failed to match branch: %w", err)
	}
	if !m {
		return false, nil
	}

	return true, nil
}

func isFieldsMatch(
	fields []string,
	filters *config.FieldOperators,
) bool {

	if filters == nil {
		return true
	}

	if filters.Exists != nil {
		if *filters.Exists && len(fields) == 0 {
			return false
		}
	}

	if filters.DoesNotExist != nil {
		if *filters.DoesNotExist && len(fields) > 0 {
			return false
		}
	}

	if filters.In != nil {
		found := false
		for _, i := range filters.In {
			if slices.Contains(fields, i) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	if filters.NotIn != nil {
		found := false
		for _, ni := range filters.NotIn {
			if slices.Contains(fields, ni) {
				found = true
				break
			}
		}
		if found {
			return false
		}
	}

	return true
}

func isFieldMatch(
	field string,
	filters *config.FieldOperators,
) (bool, error) {

	if filters == nil {
		return true, nil
	}

	if filters.Exists != nil {
		if *filters.Exists && field == "" {
			return false, nil
		}
	}

	if filters.DoesNotExist != nil {
		if *filters.DoesNotExist && field != "" {
			return false, nil
		}
	}

	if filters.In != nil {
		found := false
		for _, i := range filters.In {
			m, err := isRegexMatch(field, i)
			if err != nil {
				return false, fmt.Errorf("failed to match regex: %w", err)
			}
			if m {
				found = true
				break
			}
		}
		if !found {
			return false, nil
		}
	}

	if filters.NotIn != nil {
		found := false
		for _, ni := range filters.NotIn {
			m, err := isRegexMatch(field, ni)
			if err != nil {
				return false, fmt.Errorf("failed to match regex: %w", err)
			}
			if m {
				found = true
				break
			}
		}
		if found {
			return false, nil
		}
	}

	return true, nil
}

func isRegexMatch(
	v string,
	p string,
) (bool, error) {
	r, err := regexp.Compile(p)
	if err != nil {
		return false, fmt.Errorf("failed to compile regex: %w", err)
	}
	return r.MatchString(v), nil
}
