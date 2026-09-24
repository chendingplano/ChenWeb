package calendarhandler

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"

	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/chendingplano/shared/go/api/EchoFactory"
)

type errorResponse struct {
	Status   bool   `json:"status"`
	ErrorMsg string `json:"error_msg"`
}

// requireAdmin authorizes calendar admin operations: an authenticated user
// who is admin/owner or holds the "admin"/"root" role.
func requireAdmin(c echo.Context, loc string) (ApiTypes.RequestContext, error) {
	rc := EchoFactory.NewFromEcho(c, loc)
	user := rc.IsAuthenticated()
	if user == nil {
		return rc, c.JSON(http.StatusUnauthorized, errorResponse{Status: false, ErrorMsg: "authentication required (" + loc + ")"})
	}
	admin := user.IsOwner || user.Admin
	if !admin {
		for _, role := range user.Roles {
			role = strings.ToLower(strings.TrimSpace(role))
			if role == "admin" || role == "root" {
				admin = true
				break
			}
		}
	}
	if !admin {
		return rc, c.JSON(http.StatusForbidden, errorResponse{Status: false, ErrorMsg: "admin access required (" + loc + ")"})
	}
	return rc, nil
}

func decodeJSON(c echo.Context, target any) error {
	return json.NewDecoder(c.Request().Body).Decode(target)
}

// -- Holiday info handlers ----------------------------------------------------

// ListHolidayInfo handles GET /api/v1/calendars/holiday-info
func ListHolidayInfo(c echo.Context) error {
	rc, err := requireAdmin(c, "CWB_CAL_001")
	if err != nil {
		rc.Close()
		return err
	}
	defer rc.Close()

	list, err := listHolidayInfo(c.Request().Context(), ApiTypes.ProjectDBHandle, strings.TrimSpace(c.QueryParam("country")))
	if err != nil {
		rc.GetLogger().Error("list holiday info failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to list holiday info (CWB_CAL_002)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "results": list, "total": len(list)})
}

// CreateHolidayInfo handles POST /api/v1/calendars/holiday-info
func CreateHolidayInfo(c echo.Context) error {
	rc, err := requireAdmin(c, "CWB_CAL_010")
	if err != nil {
		rc.Close()
		return err
	}
	defer rc.Close()

	var payload HolidayInfo
	if err := decodeJSON(c, &payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_CAL_011)"})
	}
	payload.Country, payload.Name = strings.TrimSpace(payload.Country), strings.TrimSpace(payload.Name)
	if payload.Country == "" || payload.Name == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "country and name are required (CWB_CAL_012)"})
	}

	created, err := createHolidayInfo(c.Request().Context(), ApiTypes.ProjectDBHandle, payload)
	if err != nil {
		return holidayInfoError(c, rc, "create holiday info failed", "CWB_CAL_013", err)
	}
	return c.JSON(http.StatusCreated, map[string]any{"status": true, "record": created})
}

// UpdateHolidayInfo handles PUT /api/v1/calendars/holiday-info/:id
func UpdateHolidayInfo(c echo.Context) error {
	rc, err := requireAdmin(c, "CWB_CAL_020")
	if err != nil {
		rc.Close()
		return err
	}
	defer rc.Close()

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_CAL_021)"})
	}
	var payload HolidayInfo
	if err := decodeJSON(c, &payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_CAL_022)"})
	}
	payload.Country, payload.Name = strings.TrimSpace(payload.Country), strings.TrimSpace(payload.Name)
	if payload.DisplaySeqno < 1 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "display_seqno must be a positive integer (CWB_CAL_023)"})
	}
	if payload.Country == "" || payload.Name == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "country and name are required (CWB_CAL_023)"})
	}

	updated, err := updateHolidayInfo(c.Request().Context(), ApiTypes.ProjectDBHandle, id, payload)
	if err != nil {
		return holidayInfoError(c, rc, "update holiday info failed", "CWB_CAL_024", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "record": updated})
}

// DeleteHolidayInfo handles DELETE /api/v1/calendars/holiday-info/:id
func DeleteHolidayInfo(c echo.Context) error {
	rc, err := requireAdmin(c, "CWB_CAL_030")
	if err != nil {
		rc.Close()
		return err
	}
	defer rc.Close()

	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_CAL_031)"})
	}
	if err := deleteHolidayInfo(c.Request().Context(), ApiTypes.ProjectDBHandle, id); err != nil {
		return holidayInfoError(c, rc, "delete holiday info failed", "CWB_CAL_032", err)
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

func holidayInfoError(c echo.Context, rc ApiTypes.RequestContext, op, loc string, err error) error {
	rc.GetLogger().Error(op, "err", err)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "holiday info not found (" + loc + ")"})
	case errors.Is(err, ErrHolidayInfoInUse):
		return c.JSON(http.StatusConflict, errorResponse{Status: false, ErrorMsg: "holiday info is still bound to a calendar date (" + loc + ")"})
	case strings.Contains(err.Error(), "23505"):
		return c.JSON(http.StatusConflict, errorResponse{Status: false, ErrorMsg: "a holiday with this country and name already exists (" + loc + ")"})
	default:
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: op + " (" + loc + ")"})
	}
}

// -- Calendar handlers ---------------------------------------------------------

func parseCalendarKey(c echo.Context) (year int, country, calendarType string, err error) {
	year, err = strconv.Atoi(c.QueryParam("year"))
	if err != nil {
		return 0, "", "", errors.New("invalid year")
	}
	country = strings.TrimSpace(c.QueryParam("country"))
	if country == "" {
		return 0, "", "", errors.New("country is required")
	}
	calendarType = strings.TrimSpace(c.QueryParam("calendar_type"))
	if calendarType == "" {
		calendarType = "holidays"
	}
	return year, country, calendarType, nil
}

// GetCalendar handles GET /api/v1/calendars?year=&country=&calendar_type=
func GetCalendar(c echo.Context) error {
	rc, err := requireAdmin(c, "CWB_CAL_100")
	if err != nil {
		rc.Close()
		return err
	}
	defer rc.Close()

	year, country, calendarType, err := parseCalendarKey(c)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: err.Error() + " (CWB_CAL_101)"})
	}
	cal, err := getCalendar(c.Request().Context(), ApiTypes.ProjectDBHandle, year, country, calendarType)
	if err != nil {
		rc.GetLogger().Error("get calendar failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to load calendar (CWB_CAL_102)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "record": cal})
}

// UpsertCalendarDates handles PUT /api/v1/calendars/dates
func UpsertCalendarDates(c echo.Context) error {
	rc, err := requireAdmin(c, "CWB_CAL_110")
	if err != nil {
		rc.Close()
		return err
	}
	defer rc.Close()

	var payload struct {
		Year          int      `json:"year"`
		Country       string   `json:"country"`
		CalendarType  string   `json:"calendar_type"`
		Dates         []string `json:"dates"`
		HolidayInfoID int64    `json:"holiday_info_id"`
	}
	if err := decodeJSON(c, &payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_CAL_111)"})
	}
	payload.Country = strings.TrimSpace(payload.Country)
	if payload.CalendarType == "" {
		payload.CalendarType = "holidays"
	}
	if payload.Year == 0 || payload.Country == "" || len(payload.Dates) == 0 || payload.HolidayInfoID == 0 {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "year, country, dates, and holiday_info_id are required (CWB_CAL_112)"})
	}

	calendarID, err := upsertCalendarDates(c.Request().Context(), ApiTypes.ProjectDBHandle, payload.Year, payload.Country, payload.CalendarType, payload.Dates, payload.HolidayInfoID)
	if err != nil {
		rc.GetLogger().Error("upsert calendar dates failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to save calendar dates (CWB_CAL_113)"})
	}
	cal, err := getCalendar(c.Request().Context(), ApiTypes.ProjectDBHandle, payload.Year, payload.Country, payload.CalendarType)
	if err != nil {
		rc.GetLogger().Error("reload calendar after upsert failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to reload calendar (CWB_CAL_114)"})
	}
	_ = calendarID
	return c.JSON(http.StatusOK, map[string]any{"status": true, "record": cal})
}

// DeleteCalendarDate handles DELETE /api/v1/calendars/:id/dates/:date
func DeleteCalendarDate(c echo.Context) error {
	rc, err := requireAdmin(c, "CWB_CAL_120")
	if err != nil {
		rc.Close()
		return err
	}
	defer rc.Close()

	calendarID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_CAL_121)"})
	}
	date := strings.TrimSpace(c.Param("date"))
	if date == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid date (CWB_CAL_122)"})
	}
	if err := deleteCalendarDate(c.Request().Context(), ApiTypes.ProjectDBHandle, calendarID, date); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "calendar date binding not found (CWB_CAL_123)"})
		}
		rc.GetLogger().Error("delete calendar date failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete calendar date (CWB_CAL_124)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// DeleteCalendar handles DELETE /api/v1/calendars/:id
func DeleteCalendar(c echo.Context) error {
	rc, err := requireAdmin(c, "CWB_CAL_130")
	if err != nil {
		rc.Close()
		return err
	}
	defer rc.Close()

	calendarID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid id (CWB_CAL_131)"})
	}
	if err := deleteCalendar(c.Request().Context(), ApiTypes.ProjectDBHandle, calendarID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return c.JSON(http.StatusNotFound, errorResponse{Status: false, ErrorMsg: "calendar not found (CWB_CAL_132)"})
		}
		rc.GetLogger().Error("delete calendar failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to delete calendar (CWB_CAL_133)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}

// -- Default country handlers ---------------------------------------------------

// GetDefaultCountry handles GET /api/v1/calendars/default-country
func GetDefaultCountry(c echo.Context) error {
	rc, err := requireAdmin(c, "CWB_CAL_200")
	if err != nil {
		rc.Close()
		return err
	}
	defer rc.Close()

	country, err := getDefaultCountry(c.Request().Context(), ApiTypes.ProjectDBHandle)
	if err != nil {
		rc.GetLogger().Error("get default country failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to load default country (CWB_CAL_201)"})
	}
	var record any
	if country != "" {
		record = map[string]string{"country": country}
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "record": record})
}

// SetDefaultCountry handles PUT /api/v1/calendars/default-country
func SetDefaultCountry(c echo.Context) error {
	rc, err := requireAdmin(c, "CWB_CAL_210")
	if err != nil {
		rc.Close()
		return err
	}
	defer rc.Close()

	var payload struct {
		Country string `json:"country"`
	}
	if err := decodeJSON(c, &payload); err != nil {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "invalid request body (CWB_CAL_211)"})
	}
	payload.Country = strings.TrimSpace(payload.Country)
	if payload.Country == "" {
		return c.JSON(http.StatusBadRequest, errorResponse{Status: false, ErrorMsg: "country is required (CWB_CAL_212)"})
	}
	if err := setDefaultCountry(c.Request().Context(), ApiTypes.ProjectDBHandle, payload.Country); err != nil {
		rc.GetLogger().Error("set default country failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to set default country (CWB_CAL_213)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true, "record": map[string]string{"country": payload.Country}})
}

// ClearDefaultCountry handles DELETE /api/v1/calendars/default-country
func ClearDefaultCountry(c echo.Context) error {
	rc, err := requireAdmin(c, "CWB_CAL_220")
	if err != nil {
		rc.Close()
		return err
	}
	defer rc.Close()

	if err := clearDefaultCountry(c.Request().Context(), ApiTypes.ProjectDBHandle); err != nil {
		rc.GetLogger().Error("clear default country failed", "err", err)
		return c.JSON(http.StatusInternalServerError, errorResponse{Status: false, ErrorMsg: "failed to clear default country (CWB_CAL_221)"})
	}
	return c.JSON(http.StatusOK, map[string]any{"status": true})
}
