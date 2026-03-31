// Package custreqloghandler provides REST API handlers for customer request logs.
//
// Endpoints:
//
//	POST /api/v1/cust_request_logs – Create a new customer request log entry
package custreqloghandler

import (
	"fmt"
	"net/http"

	"github.com/chendingplano/deepdoc/server/api/appdatastores"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
	"github.com/labstack/echo/v4"
)

const tableName = "cust_request_logs"

// createRequest holds the payload for POST /api/v1/cust_request_logs.
type createRequest struct {
	CustName  string `json:"cust_name"`
	CustEmail string `json:"cust_email"`
	Desc      string `json:"description"`
	Purpose   string `json:"purpose"`
}

type createResponse struct {
	Status bool                               `json:"status"`
	Result *appdatastores.TableCustRequestLogDef `json:"result"`
}

type errorResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error_msg"`
}

// CreateCustRequestLog handles POST /api/v1/cust_request_logs.
// Reads cust_name, cust_email, description, purpose from the request body and inserts a record.
func CreateCustRequestLog(c echo.Context) error {
	rc := EchoFactory.NewFromEcho(c, "CWB_CRL_041")
	defer rc.Close()
	logger := rc.GetLogger()
	logger.Info("CreateCustRequestLog called")

	var req createRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("invalid request body (CWB_CRL_051): %s", err.Error()),
		})
	}

	if req.CustName == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "cust_name is required (CWB_CRL_057)",
		})
	}
	if req.CustEmail == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{
			Status:   false,
			ErrorMsg: "cust_email is required (CWB_CRL_063)",
		})
	}

	record := appdatastores.TableCustRequestLogDef{
		CustName:  req.CustName,
		CustEmail: req.CustEmail,
		Desc:      req.Desc,
		Purpose:   req.Purpose,
	}

	db := ApiTypes.ProjectDBHandle
	newID, err := appdatastores.InsertCustRequestLog(db, tableName, record)
	if err != nil {
		logger.Error("InsertCustRequestLog failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{
			Status:   false,
			ErrorMsg: fmt.Sprintf("failed to create cust_request_log (CWB_CRL_078): %s", err.Error()),
		})
	}

	record.ID = newID
	logger.Info("CreateCustRequestLog success", "id", newID)
	return c.JSON(http.StatusCreated, createResponse{Status: true, Result: &record})
}
