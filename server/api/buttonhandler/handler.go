package buttonhandler

import (
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

func HandleButtonClick(c echo.Context) error {
	fmt.Println("Button click handler called!")
	return c.JSON(http.StatusOK, map[string]string{
		"message": "Button clicked successfully (013)!",
		"status":  "success",
	})
}
