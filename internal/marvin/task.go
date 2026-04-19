package marvin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/event"
)

const (
	baseURL               = "https://serv.amazingmarvin.com"
	addTaskPOSTEndpoint   = baseURL + "/api/addTask"
	categoriesGETEndpoint = baseURL + "/api/categories"
	labelsGETEndpoint     = baseURL + "/api/labels"
	testPOSTEndpoint      = baseURL + "/api/test"
	// As per Marvin API rate limits documentation: https://github.com/amazingmarvin/MarvinAPI/wiki/Rate-limits
	defaultDailyMax = 1440
)

// AddTaskRequest is the JSON body for POST /api/addTask (Marvin OpenAPI CreateTaskRequest).
// Title autocomplete is controlled with the X-Auto-Complete HTTP header, not a JSON field.
type addTaskRequest struct {
	Title            string   `json:"title"`
	Done             *bool    `json:"done,omitempty"`
	Day              string   `json:"day,omitempty"`
	ParentID         string   `json:"parentId,omitempty"`
	LabelIDs         []string `json:"labelIds,omitempty"`
	FirstScheduled   string   `json:"firstScheduled,omitempty"`
	Rank             *float64 `json:"rank,omitempty"`
	DailySection     string   `json:"dailySection,omitempty"`
	BonusSection     string   `json:"bonusSection,omitempty"`
	CustomSection    string   `json:"customSection,omitempty"`
	TimeBlockSection string   `json:"timeBlockSection,omitempty"`
	Note             string   `json:"note,omitempty"`
	DueDate          string   `json:"dueDate,omitempty"`
	TimeEstimate     *int64   `json:"timeEstimate,omitempty"`
	IsReward         *bool    `json:"isReward,omitempty"`
	IsStarred        *int     `json:"isStarred,omitempty"`
	IsFrogged        *int     `json:"isFrogged,omitempty"`
	PlannedWeek      string   `json:"plannedWeek,omitempty"`
	PlannedMonth     string   `json:"plannedMonth,omitempty"`
	RewardPoints     *float64 `json:"rewardPoints,omitempty"`
	RewardID         string   `json:"rewardId,omitempty"`
	Backburner       *bool    `json:"backburner,omitempty"`
	ReviewDate       string   `json:"reviewDate,omitempty"`
	ItemSnoozeTime   *int64   `json:"itemSnoozeTime,omitempty"`
	PermaSnoozeTime  string   `json:"permaSnoozeTime,omitempty"`
	TimeZoneOffset   *int     `json:"timeZoneOffset,omitempty"`
}

func (m *marvin) AddTask(
	ctx context.Context,
	marvinConfig *config.MarvinConfig,
	event *event.Event,
) error {

	// Throttle requests to 1 per second as per Marvin API rate limits.
	if err := m.perSecond.Wait(ctx); err != nil {
		return fmt.Errorf("wait for per second rate limit: %w", err)
	}

	// Throttle requests to 1440 per day as per Marvin API rate limits.
	if err := m.consumeDailyBudget(); err != nil {
		return err
	}

	title, note, err := renderTemplates(
		marvinConfig.Task.TitleTemplate,
		marvinConfig.Task.NoteTemplate,
		event,
	)
	if err != nil {
		return fmt.Errorf("render templates: %w", err)
	}

	parentID, err := m.resolveParentID(marvinConfig)
	if err != nil {
		return fmt.Errorf("resolve parent ID: %w", err)
	}

	labelIDs, err := m.resolveLabelIDs(marvinConfig)
	if err != nil {
		return fmt.Errorf("resolve label IDs: %w", err)
	}

	req := buildAddTaskRequest(title, note, parentID, labelIDs, &marvinConfig.Task)

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, addTaskPOSTEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Token", *m.apiToken)

	// auto-complete is set to true by default, so we only need to set it if it's false
	autoComplete := marvinConfig.AutoComplete == nil || *marvinConfig.AutoComplete
	httpReq.Header.Set(config.MarvinTitleAutoCompleteHeader, strconv.FormatBool(autoComplete))

	resp, err := m.client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("POST addTask: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return &APIError{
			StatusCode: resp.StatusCode,
			Body:       string(respBody),
		}
	}

	return nil
}

// resolveParentID returns the Marvin parentId for the task. ListID takes
// precedence; if empty, ListName is resolved via GET /api/categories.
func (m *marvin) resolveParentID(cfg *config.MarvinConfig) (string, error) {
	if cfg.ListID != "" {
		return cfg.ListID, nil
	}
	if cfg.ListName == "" {
		return "", nil
	}
	// TODO: GET /api/categories, match by title, return _id
	return "", fmt.Errorf("list_name resolution not yet implemented")
}

// resolveLabelIDs merges explicit LabelIDs with any LabelNames resolved via
// GET /api/labels.
func (m *marvin) resolveLabelIDs(cfg *config.MarvinConfig) ([]string, error) {
	ids := make([]string, len(cfg.LabelIDs))
	copy(ids, cfg.LabelIDs)

	if len(cfg.LabelNames) == 0 {
		return ids, nil
	}
	// TODO: GET /api/labels, match by title, append resolved IDs
	return nil, fmt.Errorf("label_names resolution not yet implemented")
}

// dailySections: values per Marvin OpenAPI spec (dailySection enum).
var dailySections = map[string]bool{
	"Morning":   true,
	"Afternoon": true,
	"Evening":   true,
}

func buildAddTaskRequest(
	title, note, parentID string,
	labelIDs []string,
	task *config.MarvinTask,
) *addTaskRequest {
	req := &addTaskRequest{
		Title:        title,
		Note:         note,
		ParentID:     parentID,
		LabelIDs:     labelIDs,
		Day:          task.Day,
		DueDate:      task.DueDate,
		PlannedWeek:  task.PlannedWeek,
		PlannedMonth: task.PlannedMonth,
		TimeEstimate: task.TimeEstimateMs,
		IsStarred:    task.Priority,
		IsFrogged:    task.Frog,
		IsReward:     task.IsReward,
		RewardPoints: task.RewardPoints,
		ReviewDate:   task.ReviewDate,
	}

	if task.Section != "" {
		if dailySections[task.Section] {
			req.DailySection = task.Section
		} else {
			req.CustomSection = task.Section
		}
	}

	return req
}

func nextUTCMidnight(now time.Time) time.Time {
	utc := now.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
}

func (m *marvin) consumeDailyBudget() error {
	m.dailyMu.Lock()
	defer m.dailyMu.Unlock()

	now := time.Now().UTC()
	if !now.Before(m.dailyResetAt) {
		m.dailyCount = 0
		m.dailyResetAt = nextUTCMidnight(now)
	}

	if m.dailyCount >= m.dailyMax {
		return &DailyBudgetExceededError{
			ResetsAt: m.dailyResetAt,
		}
	}

	m.dailyCount++
	return nil
}
