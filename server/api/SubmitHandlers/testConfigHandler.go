package SubmitHandlers

import (
	"fmt"
	"log"
	"net/http"
	"github.com/labstack/echo/v4"
	"github.com/chendingplano/deepdoc/server/api/Databases"
	"github.com/chendingplano/deepdoc/server/api/DataStructures"

)


// HandleConfigSubmission handles the config form submission
func HandleConfigSubmission(c echo.Context) error {
	// Bind the request body to our struct
	customer_data := new(DataStructures.AosCustomer)
	if err := c.Bind(customer_data); err != nil {
		log.Printf("Bind error (MID_001_019): %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request data (019)",
		})
	}

	// Process the data (in a real application, you might save to database)
	fmt.Printf("Received config  %+v\n", customer_data)
	
	// Validate required fields
	if customer_data.UserName == "" || customer_data.Email == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "UserName and Email are required (029)",
		})
	}

	ctx := c.Request().Context()

	err := Databases.AosInsertCustomerData(ctx, "miner", "customers", customer_data)
	if err != nil {
		log.Printf("***** Alarm Failed insert to table (MID_001_039): %v", err)
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Failed inserting to db (038)",
		})
	}

	// For now, just return success
	return c.JSON(http.StatusOK, map[string]string{
		"message": "customer received successfully (044)",
		"status":  "success",
	})
}
