package datasync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

// fetchSourceItemDefinitions calls another ChenWeb instance's discovery
// endpoint (HandleListSourceItems) to learn what sync items it can act as a
// source for -- used to write-through-cache items this target doesn't
// already have a local definition for (design.md Decision 4).
func fetchSourceItemDefinitions(ctx context.Context, sourceURL, sharedSecret string) ([]TableSyncItem, error) {
	base := strings.TrimRight(sourceURL, "/") + "/api/internal/data-sync/items"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base, nil)
	if err != nil {
		return nil, fmt.Errorf("build discovery request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+sharedSecret)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("data sync source unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("data sync source returned %s", resp.Status)
	}

	var out struct {
		Items []itemDefinition `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode discovery response: %w", err)
	}
	items := make([]TableSyncItem, len(out.Items))
	for i, def := range out.Items {
		items[i] = fromItemDefinition(def)
	}
	return items, nil
}

// fileTransferClient has no fixed timeout, unlike httpClient's 30s (which
// bounds a whole request including the body read, too short for a large
// video file over a slow link) -- the caller's context is what bounds a
// file fetch instead.
var fileTransferClient = &http.Client{}

// fetchItemFile calls another ChenWeb instance's per-file endpoint
// (HandleGetItemFile) to stream one row's file, identified by its natural
// key (in the item's NaturalKey order). The caller must Close() the
// returned body.
func fetchItemFile(ctx context.Context, sourceURL, sharedSecret, itemID string, naturalKeyValues []string) (io.ReadCloser, error) {
	keyJSON, err := json.Marshal(naturalKeyValues)
	if err != nil {
		return nil, fmt.Errorf("encode file key: %w", err)
	}
	base := strings.TrimRight(sourceURL, "/") + "/api/internal/data-sync/items/" + url.PathEscape(itemID) + "/files"
	q := url.Values{}
	q.Set("key", string(keyJSON))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("build file request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+sharedSecret)

	resp, err := fileTransferClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("data sync source unreachable: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("data sync source returned %s fetching file", resp.Status)
	}
	return resp.Body, nil
}
