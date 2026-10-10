package kbhandler

import (
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

// goldMetricOption is one (skill version, model) combination present in
// testbed.metrics, with how many documents and benchmark runs it covers.
type goldMetricOption struct {
	SkillVersion string `json:"skill_version"`
	ModelName    string `json:"model_name"`
	RecordCount  int    `json:"record_count"`
	RunCount     int    `json:"run_count"`
}

type goldMetricOptionsResponse struct {
	Status bool `json:"status"`
	// SkillVersions is newest first; the Gold Metrics view defaults to the first.
	SkillVersions []string           `json:"skill_versions"`
	Options       []goldMetricOption `json:"options"`
}

const goldMetricOptionsQuery = `
SELECT skill_version, model_name,
       COUNT(DISTINCT input_record_id) AS record_count,
       COUNT(DISTINCT run_id) AS run_count
FROM testbed.metrics
GROUP BY skill_version, model_name
`

// compareSkillVersions orders dotted versions such as "1.10.0" numerically,
// segment by segment; a non-numeric segment falls back to string comparison.
// It returns <0, 0 or >0 like strings.Compare.
func compareSkillVersions(a, b string) int {
	as := strings.Split(strings.TrimPrefix(strings.TrimSpace(a), "v"), ".")
	bs := strings.Split(strings.TrimPrefix(strings.TrimSpace(b), "v"), ".")
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y string
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		xn, xErr := strconv.Atoi(x)
		yn, yErr := strconv.Atoi(y)
		if x == "" {
			xn, xErr = 0, nil
		}
		if y == "" {
			yn, yErr = 0, nil
		}
		if xErr == nil && yErr == nil {
			if xn != yn {
				if xn < yn {
					return -1
				}
				return 1
			}
			continue
		}
		if c := strings.Compare(x, y); c != 0 {
			return c
		}
	}
	return 0
}

// sortSkillVersionsDesc sorts versions newest first.
func sortSkillVersionsDesc(versions []string) {
	sort.SliceStable(versions, func(i, j int) bool {
		return compareSkillVersions(versions[i], versions[j]) > 0
	})
}

// newestGoldSkillVersion returns the newest skill_version in testbed.metrics,
// or "" when the table is empty.
func newestGoldSkillVersion(db *sql.DB) (string, error) {
	rows, err := db.Query(`SELECT DISTINCT skill_version FROM testbed.metrics`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	versions := make([]string, 0)
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return "", err
		}
		versions = append(versions, v)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(versions) == 0 {
		return "", nil
	}
	sortSkillVersionsDesc(versions)
	return versions[0], nil
}

// ListGoldMetricOptions handles GET /api/v1/kb/metrics/gold-options: the skill
// versions and models available for the Gold Metrics view's filters.
func ListGoldMetricOptions(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_KB_M_040")
	defer rc.Close()
	logger := rc.GetLogger()

	rows, err := ApiTypes.ProjectDBHandle.Query(goldMetricOptionsQuery)
	if err != nil {
		logger.Error("query gold metric options failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to list gold metric options (CWB_KB_M_030)",
		})
	}
	defer rows.Close()

	options := make([]goldMetricOption, 0)
	seen := make(map[string]bool)
	versions := make([]string, 0)
	for rows.Next() {
		var o goldMetricOption
		if err := rows.Scan(&o.SkillVersion, &o.ModelName, &o.RecordCount, &o.RunCount); err != nil {
			logger.Error("scan gold metric option failed", "err", err)
			return c.JSON(http.StatusInternalServerError, errorResponse{
				Status:   false,
				ErrorMsg: "failed to scan gold metric options (CWB_KB_M_031)",
			})
		}
		options = append(options, o)
		if !seen[o.SkillVersion] {
			seen[o.SkillVersion] = true
			versions = append(versions, o.SkillVersion)
		}
	}
	if err := rows.Err(); err != nil {
		logger.Error("iterate gold metric options failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: "failed to iterate gold metric options (CWB_KB_M_032)",
		})
	}
	sortSkillVersionsDesc(versions)
	sort.SliceStable(options, func(i, j int) bool {
		if c := compareSkillVersions(options[i].SkillVersion, options[j].SkillVersion); c != 0 {
			return c > 0
		}
		return options[i].ModelName < options[j].ModelName
	})
	logger.Info("listed gold metric options", "versions", len(versions), "options", len(options))

	return c.JSON(http.StatusOK, goldMetricOptionsResponse{
		Status:        true,
		SkillVersions: versions,
		Options:       options,
	})
}

// attachGoldMetricRuns annotates only the authorized, paginated input list with
// its matching benchmark run/version/model identities, without duplicating documents.
func attachGoldMetricRuns(db *sql.DB, records []inputRecord, filters listInputsFilters) error {
	if len(records) == 0 {
		return nil
	}
	args := make([]any, 0, len(records)+2)
	placeholders := make([]string, 0, len(records))
	indices := make(map[int64]int, len(records))
	for i, record := range records {
		args = append(args, record.ID)
		placeholders = append(placeholders, fmt.Sprintf("$%d", len(args)))
		indices[record.ID] = i
	}
	where := "input_record_id IN (" + strings.Join(placeholders, ", ") + ")"
	for _, filter := range []struct{ column, value string }{
		{"skill_version", filters.GoldSkillVersion}, {"model_name", filters.GoldModelName},
	} {
		if value := strings.TrimSpace(filter.value); value != "" {
			args = append(args, value)
			where += fmt.Sprintf(" AND %s = $%d", filter.column, len(args))
		}
	}
	rows, err := db.Query("SELECT DISTINCT input_record_id, skill_version, model_name, run_id FROM testbed.metrics WHERE "+where+" ORDER BY input_record_id, skill_version, model_name, run_id", args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var run goldMetricRun
		if err := rows.Scan(&id, &run.SkillVersion, &run.ModelName, &run.RunID); err != nil {
			return err
		}
		if i, ok := indices[id]; ok {
			records[i].GoldMetricRuns = append(records[i].GoldMetricRuns, run)
		}
	}
	return rows.Err()
}

// goldBenchmarkInputTable lists one row per input/run/version/model. DISTINCT
// collapses metric rows within a run while preserving repeated benchmarks.
// inputTable comes from resolveInputTable, never from a client-supplied name.
func goldBenchmarkInputTable(inputTable string) string {
	return fmt.Sprintf(`(SELECT input.*, benchmark.skill_version AS benchmark_skill_version,
		benchmark.model_name AS benchmark_model_name, benchmark.run_id AS benchmark_run_id
		FROM %s input JOIN (
			SELECT DISTINCT input_record_id, skill_version, model_name, run_id FROM testbed.metrics
		) benchmark ON benchmark.input_record_id = input.id)`, inputTable)
}
