package EchartData

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/chendingplano/deepdoc/server/api/Databases"
	"github.com/labstack/echo/v4"
)

// HandleConfigSubmission handles the config form submission
func RetrieveDataForEchartDemo01(c echo.Context) error {
	// Process the data (in a real application, you might save to database)
	fmt.Printf("To retrieve data for Echart Demo01")
	
	// ctx := c.Request().Context()

	log.Printf("***** Alarm Not implemented yet(MID_001_039)")
	return c.JSON(http.StatusBadRequest, map[string]string{
		"error": "Failed inserting to db (038)",
	})

	// For now, just return success
	// return c.JSON(http.StatusOK, map[string]string{
	// 	"message": "customer received successfully (044)",
	// 	"status":  "success",
	// })
}

// HandleConfigSubmission handles the config form submission
func RetrieveDataForEchartDemo02(c echo.Context) error {
	// Process the data (in a real application, you might save to database)
	fmt.Printf("To retrieve data for Echart Demo02")
	
	// ctx := c.Request().Context()

	log.Printf("***** Alarm Not implemented yet(MID_001_039)")
	return c.JSON(http.StatusBadRequest, map[string]string{
		"error": "Failed inserting to db (038)",
	})

	// For now, just return success
	// return c.JSON(http.StatusOK, map[string]string{
	// 	"message": "customer received successfully (044)",
	// 	"status":  "success",
	// })
}

// HandleConfigSubmission handles the config form submission
func RetrieveDataForEchartDemo03(c echo.Context) error {
	// Process the data (in a real application, you might save to database)
	fmt.Printf("To retrieve data for Echart Demo03")
	
	// ctx := c.Request().Context()

	log.Printf("***** Alarm Not implemented yet(MID_001_039)")
	return c.JSON(http.StatusBadRequest, map[string]string{
		"error": "Failed inserting to db (038)",
	})

	// For now, just return success
	// return c.JSON(http.StatusOK, map[string]string{
	// 	"message": "customer received successfully (044)",
	// 	"status":  "success",
	// })
}

// contains checks if a string slice contains a specific string
func contains(slice []string, val string) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

func RetrieveDataForEchartDemo04(c echo.Context) error {
	// Process the data (in a real application, you might save to database)
	fmt.Printf("To retrieve data for Echart Demo04 (MID_001_073)")
	
	type ProcessStatus struct {
		Type 		string  `json:"type"`
		Status		string  `json:"status"`
		RcdCount	int		`json:"rcd_count"`
		CreateAt	time.Time `json:"create_at"`
	}

	// Query the database for dashboard data

	stmt := `SELECT 
				IFNULL(status_name, ''),
				IFNULL(status_value, ''),
				IFNULL(rcd_count, 0),
				created_at FROM process_status order by created_at desc LIMIT 3000`

	log.Printf("To retrieve data for dashboard01:%s (MID_001_022)", stmt)
	rows, err := Databases.MySql_DB_miner.Query(stmt)
	if err != nil {
		log.Printf("DB error (MID_ED3_090): %v", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to retrieve data from database (MID_ED3_092)",
		})
	}
	defer rows.Close()

	var scanned_results []ProcessStatus

	for rows.Next() {
		var row ProcessStatus

		// Use sql.NullTime for the datetime field to handle NULLs properly
        var createAt sql.NullTime

		if err := rows.Scan(&row.Type, 
						&row.Status, 
						&row.RcdCount,
						&createAt); err != nil {
			log.Printf("Row scan error: %v (MID_ED3_096)", err)
			continue
		}

		if row.Status == "" {
			row.Status = "empty"
		}

		// Handle NULL datetime
        if createAt.Valid {
            row.CreateAt = createAt.Time
			scanned_results = append(scanned_results, row)
        } else {
			log.Printf("Invalid datetime value (MID_ED3_133), type:%s, status:%s, count:%d",
			 	row.Type, row.Status, row.RcdCount)
			continue
        }
	}

	if err := rows.Err(); err != nil {
		log.Printf("Rows error: %v (MID_001_046)", err)
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Error reading data from database (MID_001_048)",
		})
	}

	type SeriesEntry struct {
		Name string  `json:"name"`
		Data []int   `json:"data"`
	}

	type ChartData struct {
		Categories []string     	`json:"categories"`
		Series     []SeriesEntry 	`json:"series"`
	}

	// Step 1: build structure
	final_results := make(map[string]*ChartData)

	for _, r := range scanned_results {
		t := r.Type

		// ensure chart entry exists
		if _, ok := final_results[t]; !ok {
			final_results[t] = &ChartData{
				Categories: []string{},
				Series:     []SeriesEntry{},
			}
		}

		cd := final_results[t]

		// append date if not already included
		dateStr := r.CreateAt.Format("2006-01-02 15:04:05")
		if !contains(cd.Categories, dateStr) {
			cd.Categories = append(cd.Categories, dateStr)
		}

		// find or create series entry for this status
		seriesIdx := -1
		for i, s := range cd.Series {
			if s.Name == r.Status {
				seriesIdx = i
				break
			}
		}
		if seriesIdx == -1 {
			cd.Series = append(cd.Series, SeriesEntry{Name: r.Status, Data: []int{r.RcdCount}})
		} else {
			cd.Series[seriesIdx].Data = append(cd.Series[seriesIdx].Data, r.RcdCount)
		}
	}

	// optional: sort categories by date
	// for _, cd := range final_results {
	// 	sort.Strings(cd.Categories)
	// }

	// Step 2: marshal to JSON
	// jsonBytes, err := json.MarshalIndent(final_results, "", "  ")
	// if err != nil {
	// 	panic(err)
	// }
	// fmt.Println(string(jsonBytes))

	log.Printf("Retrieved %d rows (MID_001_052)", len(scanned_results))
	return c.JSON(http.StatusOK, final_results)
}