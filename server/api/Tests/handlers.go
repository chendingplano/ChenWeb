package Test_FormSubmitHandler

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

// ConfigData represents the form data structure
type ConfigData struct {
	UserName      string `json:"userName" form:"userName"`
	DateOfBirth   string `json:"dateOfBirth" form:"dateOfBirth"`
	Email         string `json:"email" form:"email"`
	PhoneNumber   string `json:"phoneNumber" form:"phoneNumber"`
	Education     string `json:"education" form:"education"`
	IsMarried     bool   `json:"isMarried" form:"isMarried"`
	NumberOfKids  int    `json:"numberOfKids" form:"numberOfKids"`
}

// HandleConfigSubmission handles the config form submission
func HandleConfigSubmission(c echo.Context) error {
	// Bind the request body to our struct
	configData := new(ConfigData)
	if err := c.Bind(configData); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request data",
		})
	}

	// Process the data (in a real application, you might save to database)
	fmt.Printf("Received config data: %+v\n", configData)
	
	// Validate required fields
	if configData.UserName == "" || configData.Email == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "UserName and Email are required",
		})
	}

	// Here you would typically:
	// 1. Validate the data further
	// 2. Save to database
	// 3. Process the information
	// 4. Return appropriate response

	// For now, just return success
	return c.JSON(http.StatusOK, map[string]string{
		"message": "Config data received successfully",
		"status":  "success",
	})
}
