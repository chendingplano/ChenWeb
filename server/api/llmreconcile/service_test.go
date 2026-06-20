package llmreconcile

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeStore struct {
	accounts          []Account
	insertedSnapshots []BalanceSnapshot
	latestSnapshots   map[string]BalanceSnapshot
	firstSnapshots    map[string]BalanceSnapshot
	upsertedReports   []ReconciledDailyReport
}

func (s *fakeStore) ListDeepSeekReconciliationAccounts(context.Context) ([]Account, error) {
	return s.accounts, nil
}

func (s *fakeStore) InsertBalanceSnapshot(_ context.Context, snap BalanceSnapshot) error {
	s.insertedSnapshots = append(s.insertedSnapshots, snap)
	return nil
}

func (s *fakeStore) LatestBalanceSnapshotForDay(_ context.Context, accountID string, workspaceDay time.Time) (BalanceSnapshot, error) {
	key := accountID + "|" + workspaceDay.Format("2006-01-02")
	snap, ok := s.latestSnapshots[key]
	if !ok {
		return BalanceSnapshot{}, os.ErrNotExist
	}
	return snap, nil
}

func (s *fakeStore) FirstBalanceSnapshotForDay(_ context.Context, accountID string, workspaceDay time.Time) (BalanceSnapshot, error) {
	key := accountID + "|" + workspaceDay.Format("2006-01-02")
	snap, ok := s.firstSnapshots[key]
	if !ok {
		return BalanceSnapshot{}, os.ErrNotExist
	}
	return snap, nil
}

func (s *fakeStore) UpsertProviderReconciledDailyReport(_ context.Context, report ReconciledDailyReport) error {
	s.upsertedReports = append(s.upsertedReports, report)
	return nil
}

type fakeBalanceFetcher struct {
	result BalanceFetchResult
}

func (f fakeBalanceFetcher) FetchBalance(context.Context, string, string) (BalanceFetchResult, error) {
	return f.result, nil
}

func TestRunnerRunCapturesSnapshotAndReconcilesYesterday(t *testing.T) {
	loc := time.FixedZone("America/Chicago", -5*60*60)
	now := time.Date(2026, 6, 20, 2, 5, 0, 0, loc)
	today := time.Date(2026, 6, 20, 0, 0, 0, 0, loc)
	yesterday := today.AddDate(0, 0, -1)
	store := &fakeStore{
		accounts: []Account{
			{
				ID:          "acct_1",
				AccountName: "DeepSeek Primary",
				Provider:    "deepseek",
				BaseURL:     "https://api.deepseek.com",
				APIKeyRef:   "sk-live-1",
			},
		},
		latestSnapshots: map[string]BalanceSnapshot{
			"acct_1|" + yesterday.Format("2006-01-02"): {
				AccountID:     "acct_1",
				WorkspaceDay:  yesterday,
				BalanceAmount: 25.50,
				CurrencyCode:  "USD",
			},
		},
		firstSnapshots: map[string]BalanceSnapshot{},
	}
	archiveRoot := t.TempDir()
	runner := &Runner{
		Store:        store,
		BalanceAPI:   fakeBalanceFetcher{result: BalanceFetchResult{BalanceAmount: 19.25, CurrencyCode: "USD", RawPayload: []byte(`{"is_available":true}`)}},
		ArchiveRoot:  archiveRoot,
		WorkspaceTZ:  loc,
		TimezoneName: "America/Chicago",
		Now: func() time.Time {
			return now
		},
	}

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(store.insertedSnapshots) != 1 {
		t.Fatalf("len(insertedSnapshots) = %d, want 1", len(store.insertedSnapshots))
	}
	snapshot := store.insertedSnapshots[0]
	if snapshot.WorkspaceDay.Format("2006-01-02") != "2026-06-20" {
		t.Fatalf("snapshot.WorkspaceDay = %s, want 2026-06-20", snapshot.WorkspaceDay.Format("2006-01-02"))
	}
	if len(store.upsertedReports) != 1 {
		t.Fatalf("len(upsertedReports) = %d, want 1", len(store.upsertedReports))
	}
	report := store.upsertedReports[0]
	if report.WorkspaceDay.Format("2006-01-02") != "2026-06-19" {
		t.Fatalf("report.WorkspaceDay = %s, want 2026-06-19", report.WorkspaceDay.Format("2006-01-02"))
	}
	if report.SpendAmount != 6.25 {
		t.Fatalf("report.SpendAmount = %v, want 6.25", report.SpendAmount)
	}
	archivePath := filepath.Join(archiveRoot, report.SourcePayloadRef)
	if _, err := os.Stat(archivePath); err != nil {
		t.Fatalf("expected archive file at %s: %v", archivePath, err)
	}
}

func TestRunnerRunUpsertsTodayReportFromFirstAndLatestSnapshot(t *testing.T) {
	loc := time.FixedZone("America/Chicago", -5*60*60)
	now := time.Date(2026, 6, 20, 11, 30, 0, 0, loc)
	today := time.Date(2026, 6, 20, 0, 0, 0, 0, loc)
	store := &fakeStore{
		accounts: []Account{
			{
				ID:          "acct_1",
				AccountName: "DeepSeek Primary",
				Provider:    "deepseek",
				BaseURL:     "https://api.deepseek.com",
				APIKeyRef:   "sk-live-1",
			},
		},
		latestSnapshots: map[string]BalanceSnapshot{},
		firstSnapshots: map[string]BalanceSnapshot{
			"acct_1|" + today.Format("2006-01-02"): {
				AccountID:     "acct_1",
				WorkspaceDay:  today,
				BalanceAmount: 500.00,
				CurrencyCode:  "CNY",
			},
		},
	}
	runner := &Runner{
		Store:        store,
		BalanceAPI:   fakeBalanceFetcher{result: BalanceFetchResult{BalanceAmount: 474.22, CurrencyCode: "CNY", RawPayload: []byte(`{"is_available":true}`)}},
		ArchiveRoot:  t.TempDir(),
		WorkspaceTZ:  loc,
		TimezoneName: "America/Chicago",
		Now: func() time.Time {
			return now
		},
	}

	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(store.upsertedReports) != 1 {
		t.Fatalf("len(upsertedReports) = %d, want 1", len(store.upsertedReports))
	}
	report := store.upsertedReports[0]
	if report.WorkspaceDay.Format("2006-01-02") != today.Format("2006-01-02") {
		t.Fatalf("report.WorkspaceDay = %s, want %s", report.WorkspaceDay.Format("2006-01-02"), today.Format("2006-01-02"))
	}
	if report.OpeningBalance != 500.00 || report.ClosingBalance != 474.22 {
		t.Fatalf("unexpected balances = %+v", report)
	}
	if math.Abs(report.SpendAmount-25.78) > 0.000001 {
		t.Fatalf("report.SpendAmount = %v, want 25.78", report.SpendAmount)
	}
}
