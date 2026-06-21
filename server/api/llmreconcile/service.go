package llmreconcile

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type AccountStore interface {
	ListDeepSeekReconciliationAccounts(ctx context.Context) ([]Account, error)
	InsertBalanceSnapshot(ctx context.Context, snap BalanceSnapshot) error
	LatestBalanceSnapshotForDay(ctx context.Context, accountID string, workspaceDay time.Time) (BalanceSnapshot, error)
	FirstBalanceSnapshotForDay(ctx context.Context, accountID string, workspaceDay time.Time) (BalanceSnapshot, error)
	UpsertProviderReconciledDailyReport(ctx context.Context, report ReconciledDailyReport) error
}

type BalanceFetcher interface {
	FetchBalance(ctx context.Context, baseURL string, apiKey string) (BalanceFetchResult, error)
}

type BalanceFetchResult struct {
	BalanceAmount float64
	CurrencyCode  string
	RawPayload    []byte
}

type Runner struct {
	Store        AccountStore
	BalanceAPI   BalanceFetcher
	ArchiveRoot  string
	WorkspaceTZ  *time.Location
	TimezoneName string
	Now          func() time.Time
}

type RunResult struct {
	AccountsConsidered int `json:"accounts_considered"`
	SnapshotsCreated   int `json:"snapshots_created"`
	ReportsReconciled  int `json:"reports_reconciled"`
}

func (r *Runner) Run(ctx context.Context) error {
	_, err := r.RunWithResult(ctx)
	return err
}

func (r *Runner) RunWithResult(ctx context.Context) (RunResult, error) {
	if r.Store == nil || r.BalanceAPI == nil {
		return RunResult{}, nil
	}

	loc := r.location()
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	capturedAt := now()
	today := startOfDay(capturedAt.In(loc))
	yesterday := today.AddDate(0, 0, -1)

	accounts, err := r.Store.ListDeepSeekReconciliationAccounts(ctx)
	if err != nil {
		return RunResult{}, err
	}
	result := RunResult{AccountsConsidered: len(accounts)}

	for _, account := range accounts {
		balance, err := r.BalanceAPI.FetchBalance(ctx, account.BaseURL, account.APIKeyRef)
		if err != nil {
			return RunResult{}, err
		}

		rawPayloadRef, err := writeBalanceArchive(r.ArchiveRoot, today, account.ID, balance.RawPayload)
		if err != nil {
			return RunResult{}, err
		}

		snapshot := BalanceSnapshot{
			AccountID:     account.ID,
			CapturedAt:    capturedAt.UTC(),
			WorkspaceDay:  today,
			BalanceAmount: balance.BalanceAmount,
			CurrencyCode:  balance.CurrencyCode,
			RawPayloadRef: rawPayloadRef,
		}
		if err := r.Store.InsertBalanceSnapshot(ctx, snapshot); err != nil {
			return RunResult{}, err
		}
		result.SnapshotsCreated++

		openingTodaySnapshot, err := r.Store.FirstBalanceSnapshotForDay(ctx, account.ID, today)
		if err != nil {
			if !isMissingSnapshot(err) {
				return RunResult{}, err
			}
		} else {
			todayReport := ReconciledDailyReport{
				AccountID:        account.ID,
				WorkspaceDay:     today,
				TimezoneName:     r.timezoneName(),
				OpeningBalance:   openingTodaySnapshot.BalanceAmount,
				ClosingBalance:   snapshot.BalanceAmount,
				SpendAmount:      openingTodaySnapshot.BalanceAmount - snapshot.BalanceAmount,
				CurrencyCode:     firstNonEmpty(balance.CurrencyCode, openingTodaySnapshot.CurrencyCode, "USD"),
				SourcePayloadRef: rawPayloadRef,
			}
			if err := r.Store.UpsertProviderReconciledDailyReport(ctx, todayReport); err != nil {
				return RunResult{}, err
			}
			result.ReportsReconciled++
		}

		openingSnapshot, err := r.Store.FirstBalanceSnapshotForDay(ctx, account.ID, yesterday)
		if err != nil {
			if isMissingSnapshot(err) {
				continue
			}
			return RunResult{}, err
		}

		closingSnapshot, err := r.Store.FirstBalanceSnapshotForDay(ctx, account.ID, today)
		if err != nil {
			if isMissingSnapshot(err) {
				closingSnapshot = snapshot
			} else {
				return RunResult{}, err
			}
		}

		report := ReconciledDailyReport{
			AccountID:        account.ID,
			WorkspaceDay:     yesterday,
			TimezoneName:     r.timezoneName(),
			OpeningBalance:   openingSnapshot.BalanceAmount,
			ClosingBalance:   closingSnapshot.BalanceAmount,
			SpendAmount:      openingSnapshot.BalanceAmount - closingSnapshot.BalanceAmount,
			CurrencyCode:     firstNonEmpty(closingSnapshot.CurrencyCode, openingSnapshot.CurrencyCode, balance.CurrencyCode, "USD"),
			SourcePayloadRef: rawPayloadRef,
		}
		if err := r.Store.UpsertProviderReconciledDailyReport(ctx, report); err != nil {
			return RunResult{}, err
		}
		result.ReportsReconciled++
	}

	return result, nil
}

func (r *Runner) location() *time.Location {
	if r.WorkspaceTZ != nil {
		return r.WorkspaceTZ
	}
	return time.UTC
}

func (r *Runner) timezoneName() string {
	if strings.TrimSpace(r.TimezoneName) != "" {
		return r.TimezoneName
	}
	return r.location().String()
}

type DeepSeekBalanceClient struct {
	HTTPClient *http.Client
}

type deepSeekBalanceResponse struct {
	BalanceInfos []struct {
		Currency     string `json:"currency"`
		TotalBalance string `json:"total_balance"`
	} `json:"balance_infos"`
}

func (c *DeepSeekBalanceClient) FetchBalance(ctx context.Context, baseURL string, apiKey string) (BalanceFetchResult, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.deepseek.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/user/balance", nil)
	if err != nil {
		return BalanceFetchResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return BalanceFetchResult{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return BalanceFetchResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return BalanceFetchResult{}, fmt.Errorf("deepseek balance request failed: status %d", resp.StatusCode)
	}

	var decoded deepSeekBalanceResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return BalanceFetchResult{}, err
	}
	if len(decoded.BalanceInfos) == 0 {
		return BalanceFetchResult{}, errors.New("deepseek balance response missing balance_infos")
	}

	selected := decoded.BalanceInfos[0]
	for _, info := range decoded.BalanceInfos {
		if strings.EqualFold(info.Currency, "USD") {
			selected = info
			break
		}
	}

	balanceAmount, err := strconv.ParseFloat(strings.TrimSpace(selected.TotalBalance), 64)
	if err != nil {
		return BalanceFetchResult{}, err
	}
	return BalanceFetchResult{
		BalanceAmount: balanceAmount,
		CurrencyCode:  firstNonEmpty(selected.Currency, "USD"),
		RawPayload:    body,
	}, nil
}

func writeBalanceArchive(root string, workspaceDay time.Time, accountID string, rawPayload []byte) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", nil
	}
	relPath := filepath.Join(
		workspaceDay.Format("2006"),
		workspaceDay.Format("2006-01"),
		workspaceDay.Format("2006-01-02"),
		"reconciliation",
		fmt.Sprintf("deepseek-account-%s-balance.json", accountID),
	)
	fullPath := filepath.Join(root, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(fullPath, rawPayload, 0o644); err != nil {
		return "", err
	}
	return filepath.Clean(relPath), nil
}

func startOfDay(ts time.Time) time.Time {
	return time.Date(ts.Year(), ts.Month(), ts.Day(), 0, 0, 0, 0, ts.Location())
}

func isMissingSnapshot(err error) bool {
	return errors.Is(err, sql.ErrNoRows) || errors.Is(err, os.ErrNotExist)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
