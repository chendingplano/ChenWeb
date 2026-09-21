package llmreconcile

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	sharedllm "github.com/chendingplano/shared/go/api/llm"
)

type AccountStore interface {
	ListDeepSeekReconciliationAccounts(ctx context.Context) ([]Account, error)
	ClaimHourlyBalanceCapture(ctx context.Context, accountID string, hour time.Time) (bool, error)
	InsertBalanceSnapshot(ctx context.Context, snap BalanceSnapshot) error
	LatestBalanceSnapshotForDay(ctx context.Context, accountID string, workspaceDay time.Time) (BalanceSnapshot, error)
	FirstBalanceSnapshotForDay(ctx context.Context, accountID string, workspaceDay time.Time) (BalanceSnapshot, error)
	UpsertProviderReconciledDailyReport(ctx context.Context, report ReconciledDailyReport) error
	RaiseBalanceFetchAlarm(ctx context.Context, accountID string, message string) error
}

type BalanceFetcher interface {
	FetchBalance(ctx context.Context, baseURL string, apiKey string) (BalanceFetchResult, error)
}

type BalanceFetchResult struct {
	BalanceAmount float64
	CurrencyCode  string
	Balances      []Balance
	RawPayload    []byte
}

type Balance struct {
	Amount       float64
	CurrencyCode string
}

type Runner struct {
	Store         AccountStore
	BalanceAPI    BalanceFetcher
	ArchiveRoot   string
	WorkspaceTZ   *time.Location
	TimezoneName  string
	Now           func() time.Time
	CaptureSource string
}

type RunResult struct {
	AccountsConsidered int              `json:"accounts_considered"`
	SnapshotsCreated   int              `json:"snapshots_created"`
	ReportsReconciled  int              `json:"reports_reconciled"`
	Failures           []AccountFailure `json:"failures,omitempty"`
}

// AccountFailure is one account's reconciliation failure within a run. It
// never aborts the run for other accounts (see RunWithResult) -- the caller
// (runReconciliationOnce) logs each one, and the failing account has already
// had an alarms_errors row raised for it.
type AccountFailure struct {
	AccountID   string `json:"account_id"`
	AccountName string `json:"account_name"`
	Err         error  `json:"-"`
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
		snapshotsCreated, reportsReconciled, err := r.processAccount(ctx, account, capturedAt, today, yesterday, loc)
		result.SnapshotsCreated += snapshotsCreated
		result.ReportsReconciled += reportsReconciled
		if err != nil {
			result.Failures = append(result.Failures, AccountFailure{AccountID: account.ID, AccountName: account.AccountName, Err: err})
			alarmMessage := fmt.Sprintf("deepseek balance reconciliation failed for account %s (%s): %v", account.AccountName, account.ID, err)
			if alarmErr := r.Store.RaiseBalanceFetchAlarm(ctx, account.ID, alarmMessage); alarmErr != nil {
				result.Failures = append(result.Failures, AccountFailure{AccountID: account.ID, AccountName: account.AccountName, Err: fmt.Errorf("failed to raise alarm: %w", alarmErr)})
			}
			continue
		}
	}

	return result, nil
}

// processAccount runs one account's hourly balance capture and daily
// reconciliation. A returned error means this account's capture failed --
// the caller (RunWithResult) alarms it and moves on to the next account
// rather than aborting the whole run, and next hour's capture slot is
// unaffected since ClaimHourlyBalanceCapture is keyed per (account, hour).
func (r *Runner) processAccount(ctx context.Context, account Account, capturedAt, today, yesterday time.Time, loc *time.Location) (snapshotsCreated int, reportsReconciled int, err error) {
	claimed, err := r.Store.ClaimHourlyBalanceCapture(ctx, account.ID, capturedAt.In(loc).Truncate(time.Hour).UTC())
	if err != nil {
		return 0, 0, err
	}
	if !claimed {
		return 0, 0, nil
	}
	balance, err := r.BalanceAPI.FetchBalance(ctx, account.BaseURL, account.APIKeyRef)
	if err != nil {
		return 0, 0, err
	}

	rawPayloadRef, err := writeBalanceArchive(r.ArchiveRoot, today, account.ID, capturedAt, balance.RawPayload)
	if err != nil {
		return 0, 0, err
	}

	balances := balance.Balances
	if len(balances) == 0 {
		balances = []Balance{{Amount: balance.BalanceAmount, CurrencyCode: balance.CurrencyCode}}
	}
	for _, currencyBalance := range balances {
		snapshot := BalanceSnapshot{
			AccountID:     account.ID,
			CapturedAt:    capturedAt.UTC(),
			WorkspaceDay:  today,
			BalanceAmount: currencyBalance.Amount,
			CurrencyCode:  currencyBalance.CurrencyCode,
			CaptureSource: r.captureSource(),
			RawPayloadRef: rawPayloadRef,
		}
		if err := r.Store.InsertBalanceSnapshot(ctx, snapshot); err != nil {
			return snapshotsCreated, reportsReconciled, err
		}
		snapshotsCreated++
	}
	snapshot := BalanceSnapshot{AccountID: account.ID, CapturedAt: capturedAt.UTC(), WorkspaceDay: today, BalanceAmount: balance.BalanceAmount, CurrencyCode: balance.CurrencyCode, CaptureSource: r.captureSource(), RawPayloadRef: rawPayloadRef}
	if len(balance.Balances) > 0 {
		snapshot.BalanceAmount = balance.Balances[0].Amount
		snapshot.CurrencyCode = balance.Balances[0].CurrencyCode
	}

	openingTodaySnapshot, err := r.Store.FirstBalanceSnapshotForDay(ctx, account.ID, today)
	if err != nil {
		if !isMissingSnapshot(err) {
			return snapshotsCreated, reportsReconciled, err
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
			return snapshotsCreated, reportsReconciled, err
		}
		reportsReconciled++
	}

	openingSnapshot, err := r.Store.FirstBalanceSnapshotForDay(ctx, account.ID, yesterday)
	if err != nil {
		if isMissingSnapshot(err) {
			return snapshotsCreated, reportsReconciled, nil
		}
		return snapshotsCreated, reportsReconciled, err
	}

	closingSnapshot, err := r.Store.FirstBalanceSnapshotForDay(ctx, account.ID, today)
	if err != nil {
		if isMissingSnapshot(err) {
			closingSnapshot = snapshot
		} else {
			return snapshotsCreated, reportsReconciled, err
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
		return snapshotsCreated, reportsReconciled, err
	}
	reportsReconciled++

	return snapshotsCreated, reportsReconciled, nil
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

func (r *Runner) captureSource() string {
	if strings.TrimSpace(r.CaptureSource) != "" {
		return r.CaptureSource
	}
	return "manual"
}

type DeepSeekBalanceClient struct {
	HTTPClient *http.Client
}

func (c *DeepSeekBalanceClient) FetchBalance(ctx context.Context, baseURL string, apiKey string) (BalanceFetchResult, error) {
	result, err := sharedllm.NewDeepSeekBalanceClient(c.HTTPClient).Fetch(ctx, baseURL, apiKey, nil)
	if err != nil {
		return BalanceFetchResult{}, err
	}
	selected := result.Balances[0]
	for _, balance := range result.Balances {
		if strings.EqualFold(balance.CurrencyCode, "USD") {
			selected = balance
			break
		}
	}
	return BalanceFetchResult{
		BalanceAmount: selected.Amount,
		CurrencyCode:  firstNonEmpty(selected.CurrencyCode, "USD"),
		Balances: func() []Balance {
			out := make([]Balance, 0, len(result.Balances))
			for _, balance := range result.Balances {
				out = append(out, Balance{Amount: balance.Amount, CurrencyCode: firstNonEmpty(balance.CurrencyCode, "USD")})
			}
			return out
		}(),
		RawPayload: result.RawPayload,
	}, nil
}

func writeBalanceArchive(root string, workspaceDay time.Time, accountID string, capturedAt time.Time, rawPayload []byte) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", nil
	}
	relPath := filepath.Join(
		workspaceDay.Format("2006"),
		workspaceDay.Format("2006-01"),
		workspaceDay.Format("2006-01-02"),
		"reconciliation",
		fmt.Sprintf("deepseek-account-%s-balance-%s.json", accountID, capturedAt.UTC().Format("20060102T150405.000000000Z")),
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
