package kbhandler

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/shared/go/api/ApiTypes"
	"github.com/labstack/echo/v4"
)

func newUploadInputsContext(t *testing.T, body *bytes.Buffer, contentType string) (echo.Context, *httptest.ResponseRecorder) {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/kb/inputs/upload", body)
	req.Header.Set(echo.HeaderContentType, contentType)
	rec := httptest.NewRecorder()
	return e.NewContext(req, rec), rec
}

func buildUploadMultipartBody(t *testing.T, fields map[string]string, files map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("WriteField(%s): %v", key, err)
		}
	}

	for name, content := range files {
		part, err := writer.CreateFormFile("files", name)
		if err != nil {
			t.Fatalf("CreateFormFile(%s): %v", name, err)
		}
		if _, err := part.Write([]byte(content)); err != nil {
			t.Fatalf("write file content: %v", err)
		}
	}

	if err := writer.Close(); err != nil {
		t.Fatalf("writer.Close: %v", err)
	}
	return body, writer.FormDataContentType()
}

func TestUploadInputsSuccess(t *testing.T) {
	stagingDir := t.TempDir()
	oldStagingDir := os.Getenv("STAGING_DIR")
	t.Setenv("STAGING_DIR", stagingDir)
	defer os.Setenv("STAGING_DIR", oldStagingDir)

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New failed: %v", err)
	}
	defer db.Close()

	oldDB := ApiTypes.ProjectDBHandle
	ApiTypes.ProjectDBHandle = db
	defer func() { ApiTypes.ProjectDBHandle = oldDB }()

	expectResolveInputTablePlural(mock)
	mock.ExpectBegin()

	insertQuery := regexp.QuoteMeta(`INSERT INTO kb.inputs (
    tenant_id,
    ks_store_id,
    requested_pipeline,
    type,
    title,
    doc_no,
    authors,
    public_info,
    private_info,
    notes,
    ks_desc,
    parser_name,
    staging_filename,
    file_name,
    md5,
    status
) VALUES (
    $1,
    $2,
    $3,
    $4,
    $5,
    $6,
    $7::jsonb,
    $8::jsonb,
    $9,
    $10,
    $11,
    $12,
    $13,
    $14,
    '[]'::jsonb
)
RETURNING id`)
	mock.ExpectQuery(insertQuery).
		WithArgs(
			"tenant-alpha",
			int64(7),
			"narrative_default",
			"pdf",
			"Doc Title",
			"DOC-42",
			"Alice;Bob",
			`"public text"`,
			`"private text"`,
			"note body",
			"store upload desc",
			"docling",
			"sample.pdf",
			sqlmock.AnyArg(),
			sqlmock.AnyArg(),
		).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(int64(81)))
	mock.ExpectCommit()

	body, contentType := buildUploadMultipartBody(t, map[string]string{
		"type":               "pdf",
		"title":              "Doc Title",
		"doc_no":             "DOC-42",
		"authors":            "Alice;Bob",
		"public_info":        "public text",
		"private_info":       "private text",
		"notes":              "note body",
		"ks_desc":            "store upload desc",
		"parser_name":        "docling",
		"ks_store_id":        "7",
		"tenant_id":          "tenant-alpha",
		"requested_pipeline": "narrative_default",
	}, map[string]string{
		"sample.pdf": "hello world",
	})

	c, rec := newUploadInputsContext(t, body, contentType)
	if err := UploadInputs(c); err != nil {
		t.Fatalf("UploadInputs returned error: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	var payload uploadInputsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Status || payload.Count != 1 || len(payload.IDs) != 1 || payload.IDs[0] != 81 {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	if _, err := os.Stat(stagingDir + "/sample.pdf"); err != nil {
		t.Fatalf("expected staged file to exist: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet db expectations: %v", err)
	}
}

func TestUploadInputsRequiresKnowledgeStore(t *testing.T) {
	t.Setenv("STAGING_DIR", t.TempDir())

	body, contentType := buildUploadMultipartBody(t, map[string]string{
		"type":        "pdf",
		"parser_name": "docling",
		"tenant_id":   "tenant-alpha",
	}, map[string]string{
		"sample.pdf": "hello world",
	})

	c, rec := newUploadInputsContext(t, body, contentType)
	if err := UploadInputs(c); err != nil {
		t.Fatalf("UploadInputs returned error: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestUploadInputsRequiresStagingDir(t *testing.T) {
	oldStagingDir, had := os.LookupEnv("STAGING_DIR")
	if had {
		defer os.Setenv("STAGING_DIR", oldStagingDir)
	} else {
		defer os.Unsetenv("STAGING_DIR")
	}
	_ = os.Unsetenv("STAGING_DIR")

	body, contentType := buildUploadMultipartBody(t, map[string]string{
		"type":        "pdf",
		"parser_name": "docling",
		"tenant_id":   "tenant-alpha",
		"ks_store_id": "7",
	}, map[string]string{
		"sample.pdf": "hello world",
	})

	c, rec := newUploadInputsContext(t, body, contentType)
	if err := UploadInputs(c); err != nil {
		t.Fatalf("UploadInputs returned error: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d, body=%s", rec.Code, rec.Body.String())
	}
}
