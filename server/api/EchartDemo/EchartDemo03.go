package EchartData

import (
	"fmt"
	"log"
	"net/http"

	"github.com/chendingplano/deepdoc/server/api/appdatastores"
	"github.com/labstack/echo/v4"
)

// HandleConfigSubmission handles the config form submission
func RetrieveDataForEchartDemo01(c echo.Context) error {
	// Process the data (in a real application, you might save to database)
	fmt.Printf("To retrieve data for Echart Demo01")
	
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
	
	type SeriesEntry struct {
		Name string  `json:"name"`
		Data []int   `json:"data"`
	}

	type ChartData struct {
		Categories []string     	`json:"categories"`
		Series     []SeriesEntry 	`json:"series"`
	}

	type ResponseStruct struct {
		Status 		bool					`json:"status"`
		NumResults 	int 					`json:"num_records"`
		ErrorMsg	string 					`json:"error_msg"`
		LOC			string					`json:"loc"`
		Results 	map[string]*ChartData	`json:"results"`
	}

	// Step 1: build structure
	// final_results := make(map[string]*ChartData)
	var final_results ResponseStruct
	final_results.Results = make(map[string]*ChartData)

	var scanned_results []appdatastores.ProcessStatus
	scanned_results, err := appdatastores.RetrieveProcessStatus()
	if err != nil {
		final_results.Status = false
		final_results.NumResults = 0
		final_results.ErrorMsg = "Failed retrieve data"
		final_results.LOC = "CWB_EDM_110"
		return c.JSON(http.StatusBadRequest, final_results)
	}

	final_results.Status = true
	final_results.NumResults = -1
	if scanned_results != nil {
		final_results.NumResults = len(scanned_results)
    	for _, r := range scanned_results {
    		t := r.Type

    		// ensure chart entry exists
    		if _, ok := final_results.Results[t]; !ok {
    			final_results.Results[t] = &ChartData{
    				Categories: []string{},
    				Series:     []SeriesEntry{},
    			}
    		}

    		cd := final_results.Results[t]

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
	}

	log.Printf("Retrieved %d rows (MID_001_052)", len(scanned_results))
	final_results.LOC = "CWB_EMD_151"
	return c.JSON(http.StatusOK, final_results)
}