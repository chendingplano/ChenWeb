package api_util

import (
	"fmt"
	"os"
)

func GetDefahotHomeURL() string {
	var url = fmt.Sprintf("%s%s", os.Getenv("APP_DOMAIN_NAME"), os.Getenv("APP_DEFAULT_ENDPOINT"))
	return url
}
