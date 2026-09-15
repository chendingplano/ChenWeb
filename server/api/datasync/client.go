package datasync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// fetchChangesPageFromSource calls another ChenWeb instance's pull endpoint
// (HandlePullChanges) to fetch one page of changes for item since cursor.
func fetchChangesPageFromSource(ctx context.Context, sourceURL, sharedSecret string, item TableSyncItem, since string) (changesResponse, error) {
	base := strings.TrimRight(sourceURL, "/") + "/api/internal/data-sync/items/" + url.PathEscape(item.ID) + "/changes"
	q := url.Values{}
	if since != "" {
		q.Set("since", since)
	}
	q.Set("limit", strconv.Itoa(defaultPullLimit))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+q.Encode(), nil)
	if err != nil {
		return changesResponse{}, fmt.Errorf("build pull request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+sharedSecret)

	resp, err := httpClient.Do(req)
	if err != nil {
		return changesResponse{}, fmt.Errorf("data sync source unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return changesResponse{}, fmt.Errorf("data sync source returned %s", resp.Status)
	}

	var out changesResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return changesResponse{}, fmt.Errorf("decode data sync source response: %w", err)
	}
	return out, nil
}

// fetchAllChanges loops over fetchChangesPageFromSource while HasMore is set,
// returning every fetched row (in order) and the final cursor to persist.
func fetchAllChanges(ctx context.Context, sourceURL, sharedSecret string, item TableSyncItem, since string) ([]Row, string, error) {
	var allRows []Row
	cursor := since
	for {
		page, err := fetchChangesPageFromSource(ctx, sourceURL, sharedSecret, item, cursor)
		if err != nil {
			return nil, "", err
		}
		allRows = append(allRows, page.Rows...)
		cursor = page.NextCursor
		if !page.HasMore {
			break
		}
	}
	return allRows, cursor, nil
}
