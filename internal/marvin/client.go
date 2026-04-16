package marvin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/mooneeb/amgi/internal/config"
	"github.com/mooneeb/amgi/internal/event"
)

const (
	baseURL               = "https://serv.amazingmarvin.com"
	addTaskPOSTEndpoint   = baseURL + "/api/addTask"
	categoriesGETEndpoint = baseURL + "/api/categories"
	labelsGETEndpoint     = baseURL + "/api/labels"
	testPOSTEndpoint      = baseURL + "/api/test"
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

type Client struct {
	logger   *slog.Logger
	apiToken string
	baseURL  string
	client   *http.Client
}

type APIError struct {
	StatusCode int
	Body       string
}

func (a *APIError) Error() string {
	return fmt.Sprintf("Status Code: %v - %s", a.StatusCode, a.Body)
}

func New(
	logger *slog.Logger,
	apiToken string,
	client *http.Client,
) *Client {
	return &Client{
		logger:   logger,
		apiToken: apiToken,
		baseURL:  baseURL,
		client:   client,
	}
}

func (c *Client) AddTask(
	marvinConfig *config.MarvinConfig,
	event *event.Event,
) error {
	title, note, err := RenderTemplates(
		marvinConfig.Task.TitleTemplate,
		marvinConfig.Task.NoteTemplate,
		event,
	)
	if err != nil {
		return fmt.Errorf("render templates: %w", err)
	}

	parentID, err := c.resolveParentID(marvinConfig)
	if err != nil {
		return fmt.Errorf("resolve parent ID: %w", err)
	}

	labelIDs, err := c.resolveLabelIDs(marvinConfig)
	if err != nil {
		return fmt.Errorf("resolve label IDs: %w", err)
	}

	req := buildAddTaskRequest(title, note, parentID, labelIDs, &marvinConfig.Task)

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequest(http.MethodPost, addTaskPOSTEndpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create HTTP request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-API-Token", c.apiToken)

	// auto-complete is set to true by default, so we only need to set it if it's false
	autoComplete := marvinConfig.AutoComplete == nil || *marvinConfig.AutoComplete
	httpReq.Header.Set(config.MarvinTitleAutoCompleteHeader, strconv.FormatBool(autoComplete))

	resp, err := c.client.Do(httpReq)
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
func (c *Client) resolveParentID(cfg *config.MarvinConfig) (string, error) {
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
func (c *Client) resolveLabelIDs(cfg *config.MarvinConfig) ([]string, error) {
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
