package codemonitor

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

type fileKind int

const (
	ignoredFile fileKind = iota
	frontendFile
	backendFile
)

// Frontend: .js .jsx .ts .tsx .svelte .css .scss .html .vue .json.
// Backend: .go .py .rs .java .kt .php .rb .sql .sh .yaml .yml .toml.
func classifyExtension(ext string) fileKind {
	switch strings.ToLower(ext) {
	case ".js", ".jsx", ".ts", ".tsx", ".svelte", ".css", ".scss", ".html", ".vue", ".json":
		return frontendFile
	case ".go", ".py", ".rs", ".java", ".kt", ".php", ".rb", ".sql", ".sh", ".yaml", ".yml", ".toml":
		return backendFile
	default:
		return ignoredFile
	}
}

func countLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	r := bufio.NewReader(f)
	n := 0
	var last byte
	seen := false
	for {
		b, readErr := r.ReadByte()
		if readErr != nil {
			if readErr != io.EOF {
				return 0, readErr
			}
			break
		}
		seen = true
		last = b
		if b == '\n' {
			n++
		}
	}
	if seen && last != '\n' {
		n++
	}
	return n, nil
}

type Snapshot struct {
	TotalLines    int `json:"total_lines"`
	FrontendFiles int `json:"frontend_files"`
	BackendFiles  int `json:"backend_files"`
}

func collectPackage(root, packagePath string) (Snapshot, error) {
	var out Snapshot
	cmd := exec.Command("git", "ls-files", "-z", "--", packagePath)
	cmd.Dir = root
	data, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("git ls-files %s: %w", packagePath, err)
	}
	for _, rel := range strings.Split(string(data), "\x00") {
		if rel == "" {
			continue
		}
		kind := classifyExtension(filepath.Ext(rel))
		if kind == ignoredFile {
			continue
		}
		lines, err := countLines(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return out, fmt.Errorf("count %s: %w", rel, err)
		}
		out.TotalLines += lines
		if kind == frontendFile {
			out.FrontendFiles++
		} else {
			out.BackendFiles++
		}
	}
	return out, nil
}

type Store struct{ DB *sql.DB }

func (s Store) Upsert(ctx context.Context, packagePath string, date time.Time, snap Snapshot) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO kb.code_monitor(package_path, collection_date, total_lines, frontend_files, backend_files, collected_at)
VALUES ($1,$2,$3,$4,$5,NOW()) ON CONFLICT (package_path, collection_date) DO UPDATE SET total_lines=EXCLUDED.total_lines, frontend_files=EXCLUDED.frontend_files, backend_files=EXCLUDED.backend_files, collected_at=NOW()`, packagePath, date.Format("2006-01-02"), snap.TotalLines, snap.FrontendFiles, snap.BackendFiles)
	return err
}

func (s Store) Collect(ctx context.Context, root string, packages []string, now time.Time) error {
	date := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var failures []string
	for _, packagePath := range packages {
		snap, err := collectPackage(root, packagePath)
		if err == nil {
			err = s.Upsert(ctx, packagePath, date, snap)
		}
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", packagePath, err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("code monitor collection failures: %s", strings.Join(failures, "; "))
	}
	return nil
}

func nextRun(now time.Time, loc *time.Location) time.Time {
	local := now.In(loc)
	next := scheduledTime(local.Year(), local.Month(), local.Day(), loc)
	if !next.After(local) {
		tomorrow := local.AddDate(0, 0, 1)
		next = scheduledTime(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), loc)
	}
	return next
}

func scheduledTime(year int, month time.Month, day int, loc *time.Location) time.Time {
	next := time.Date(year, month, day, 2, 0, 0, 0, loc)
	// Some zones skip 02:00 during the spring DST transition. Schedule at
	// the first valid local time after the missing hour (normally 03:00).
	if next.In(loc).Hour() != 2 || next.In(loc).Day() != day {
		for i := 0; i < 4*60; i++ {
			candidate := next.Add(time.Duration(i) * time.Minute)
			if candidate.In(loc).Year() == year && candidate.In(loc).YearDay() == time.Date(year, month, day, 12, 0, 0, 0, loc).YearDay() && candidate.In(loc).Hour() >= 2 {
				next = time.Date(candidate.In(loc).Year(), candidate.In(loc).Month(), candidate.In(loc).Day(), candidate.In(loc).Hour(), candidate.In(loc).Minute(), 0, 0, loc)
				break
			}
		}
	}
	return next
}

func Start(ctx context.Context, store Store, root string, packages []string, loc *time.Location, logger ApiTypes.JimoLogger) {
	go func() {
		for {
			delay := time.Until(nextRun(time.Now(), loc))
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			if err := store.Collect(ctx, root, packages, time.Now().In(loc)); err != nil {
				logger.Error("code monitor collection failed", "error", err)
			}
		}
	}()
}

type row struct {
	Date          string `json:"collection_date"`
	TotalLines    int    `json:"total_lines"`
	FrontendFiles int    `json:"frontend_files"`
	BackendFiles  int    `json:"backend_files"`
}
type packageRows struct {
	PackagePath string `json:"package_path"`
	Rows        []row  `json:"rows"`
}
type response struct {
	Packages []packageRows `json:"packages"`
}

func dateRange(r string, now time.Time, loc *time.Location) (time.Time, time.Time, error) {
	today := time.Date(now.In(loc).Year(), now.In(loc).Month(), now.In(loc).Day(), 0, 0, 0, 0, loc)
	start := today
	switch r {
	case "today":
	case "yesterday":
		start = today.AddDate(0, 0, -1)
	case "last_7_days":
		start = today.AddDate(0, 0, -6)
	case "last_month":
		start = today.AddDate(0, 0, -29)
	case "last_6_months":
		start = clampMonth(today, -6)
	case "last_12_months":
		start = clampMonth(today, -12)
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("invalid range")
	}
	return start, today, nil
}

func clampMonth(day time.Time, months int) time.Time {
	first := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, day.Location()).AddDate(0, months, 0)
	lastDay := time.Date(first.Year(), first.Month()+1, 0, 0, 0, 0, 0, day.Location()).Day()
	d := day.Day()
	if d > lastDay {
		d = lastDay
	}
	return time.Date(first.Year(), first.Month(), d, 0, 0, 0, 0, day.Location())
}

func Handler(packages []string, loc *time.Location) echo.HandlerFunc {
	return func(c echo.Context) error {
		db := ApiTypes.ProjectDBHandle
		if db == nil {
			return echo.NewHTTPError(503, "database unavailable")
		}
		rng := c.QueryParam("range")
		if rng == "" {
			return echo.NewHTTPError(400, "range is required")
		}
		start, end, err := dateRange(rng, time.Now(), loc)
		if err != nil {
			return echo.NewHTTPError(400, "invalid range")
		}
		out := response{Packages: make([]packageRows, 0, len(packages))}
		for _, pkg := range packages {
			p := packageRows{PackagePath: pkg, Rows: []row{}}
			rows, err := db.QueryContext(c.Request().Context(), `SELECT collection_date::text,total_lines,frontend_files,backend_files FROM kb.code_monitor WHERE package_path=$1 AND collection_date BETWEEN $2 AND $3 ORDER BY collection_date`, pkg, start.Format("2006-01-02"), end.Format("2006-01-02"))
			if err != nil {
				return err
			}
			for rows.Next() {
				var v row
				if err := rows.Scan(&v.Date, &v.TotalLines, &v.FrontendFiles, &v.BackendFiles); err != nil {
					rows.Close()
					return err
				}
				p.Rows = append(p.Rows, v)
			}
			if err := rows.Err(); err != nil {
				rows.Close()
				return err
			}
			rows.Close()
			out.Packages = append(out.Packages, p)
		}
		return c.JSON(200, out)
	}
}
