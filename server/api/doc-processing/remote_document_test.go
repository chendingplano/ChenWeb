package docprocessing

import (
	"context"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRemoteDocumentAdmission(t *testing.T) {
	for _, tc := range []struct {
		name, origin  string
		marked, allow bool
		want          error
	}{
		{"local", "local", false, false, nil},
		{"remote without confirmation", "remote", false, false, errUnconfirmedRemoteDocument},
		{"remote confirmed", "remote", false, true, nil},
		{"marked local", "local", true, false, ErrDocumentBusy},
		{"marked remote confirmed", "remote", true, true, ErrDocumentBusy},
	} {
		t.Run(tc.name, func(t *testing.T) {
			db, mock, err := sqlmock.New()
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			mock.ExpectQuery("SELECT i.doc_origin, EXISTS").WithArgs(int64(42)).
				WillReturnRows(sqlmock.NewRows([]string{"doc_origin", "marked"}).AddRow(tc.origin, tc.marked))
			s := ControlService{LeaseDB: db}
			if err := s.checkDocumentAdmission(context.Background(), 42, tc.allow); !errors.Is(err, tc.want) {
				t.Fatalf("admission error = %v, want %v", err, tc.want)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRemoteDocumentConfirmationMustBeJSONBoolean(t *testing.T) {
	for _, tc := range []struct {
		payload string
		want    bool
	}{
		{`{"record_id":"42"}`, false},
		{`{"record_id":"42","allow_remote_doc":true}`, true},
		{`{"record_id":"42","allow_remote_doc":"true"}`, false},
		{`{"record_id":"42","allow_remote_doc":false}`, false},
	} {
		payload := []byte(tc.payload)
		line, err := ParseLineFileGeneratedEvent(payload)
		if err != nil || line.AllowRemoteDoc != tc.want {
			t.Fatalf("line event: got %+v, %v", line, err)
		}
		start, err := ParseStartDocProcessingEvent(payload)
		if err != nil || start.AllowRemoteDoc != tc.want {
			t.Fatalf("start event: got %+v, %v", start, err)
		}
	}
}
