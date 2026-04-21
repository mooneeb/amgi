package marvin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// category is the Marvin /api/categories response shape (subset of fields we use).
type category struct {
	ID       string `json:"_id"`
	Title    string `json:"title"`
	ParentID string `json:"parentId"`
}

// listCategories fetches all categories from GET /api/categories.
// Rate-limited by the reads limiter (1 req per 3 seconds per Marvin API docs).
func (m *marvin) listCategories(ctx context.Context) ([]category, error) {
	if err := m.reads.Wait(ctx); err != nil {
		return nil, fmt.Errorf("wait for reads rate limit: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.baseURL+"/api/categories", nil)
	if err != nil {
		return nil, fmt.Errorf("create GET request: %w", err)
	}
	req.Header.Set("X-API-Token", *m.apiToken)

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /api/categories: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, &APIError{StatusCode: resp.StatusCode, Body: string(body)}
	}

	var cats []category
	if err := json.NewDecoder(resp.Body).Decode(&cats); err != nil {
		return nil, fmt.Errorf("decode categories: %w", err)
	}

	return cats, nil
}
