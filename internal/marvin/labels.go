package marvin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// label is the Marvin /api/labels response shape (subset of fields we use).
type label struct {
	ID      string `json:"_id"`
	Title   string `json:"title"`
	GroupID string `json:"groupId"`
}

// listLabels fetches all labels from GET /api/labels.
// Rate-limited by the reads limiter (1 req per 3 seconds per Marvin API docs).
func (m *marvin) listLabels(ctx context.Context) ([]label, error) {
	if err := m.reads.Wait(ctx); err != nil {
		return nil, fmt.Errorf("wait for reads rate limit: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.baseURL+"/api/labels", nil)
	if err != nil {
		return nil, fmt.Errorf("create GET request: %w", err)
	}
	req.Header.Set("X-API-Token", *m.apiToken)

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /api/labels: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var lbls []label
	if err := json.NewDecoder(resp.Body).Decode(&lbls); err != nil {
		return nil, fmt.Errorf("decode labels: %w", err)
	}

	return lbls, nil
}
