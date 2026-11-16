package processhealthalarms

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/chendingplano/deepdoc/server/processhealth"
)

func TestSyncIncidentAndRecovery(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Now().UTC()
	in := &processhealth.Incident{Key: "doc-processor/missing_config", Service: "doc-processor", Reason: "missing_config", Since: now, LastNotified: now, Occurrences: 1}
	state := processhealth.NewMonitorState()
	state.Incidents[in.Key] = in
	path := filepath.Join(t.TempDir(), "state.json")
	save := func() {
		t.Helper()
		b, e := json.Marshal(state)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(path, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	save()
	scope := "process-health/" + in.Key + "/" + now.Format(time.RFC3339Nano)
	mock.ExpectExec(regexp.QuoteMeta(upsertIncident)).WithArgs("error", sqlmock.AnyArg(), "unsolved", scope, now, now).WillReturnResult(sqlmock.NewResult(1, 1))
	if err := Sync(context.Background(), db, path); err != nil {
		t.Fatal(err)
	}
	recovered := now.Add(time.Minute)
	in.ResolvedAt = &recovered
	save()
	mock.ExpectExec(regexp.QuoteMeta(upsertIncident)).WithArgs("info", sqlmock.AnyArg(), "solved", scope, now, recovered).WillReturnResult(sqlmock.NewResult(0, 1))
	if err := Sync(context.Background(), db, path); err != nil {
		t.Fatal(err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestSyncMissingAndInvalidState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := Sync(context.Background(), nil, path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Sync(context.Background(), nil, path); err == nil {
		t.Fatal("invalid state was accepted")
	}
}

func TestSyncRetainsClosedIncidentWhenFaultRecursWhileBackendOffline(t *testing.T) {
 db,mock,err:=sqlmock.New();if err!=nil{t.Fatal(err)};defer db.Close()
 now:=time.Now().UTC();closed:=now.Add(time.Minute)
 past:=processhealth.Incident{Key:"same-fault",Service:"worker",Reason:"config_invalid",Since:now,LastNotified:now,ResolvedAt:&closed}
 current:=past;current.Since=now.Add(2*time.Minute);current.LastNotified=current.Since;current.ResolvedAt=nil
 state:=processhealth.NewMonitorState();state.History=[]processhealth.Incident{past};state.Incidents[current.Key]=&current
 data,err:=json.Marshal(state);if err!=nil{t.Fatal(err)}
 path:=filepath.Join(t.TempDir(),"state.json");if err=os.WriteFile(path,data,0600);err!=nil{t.Fatal(err)}
 mock.ExpectExec(regexp.QuoteMeta(upsertIncident)).WithArgs("info",sqlmock.AnyArg(),"solved","process-health/same-fault/"+now.Format(time.RFC3339Nano),now,closed).WillReturnResult(sqlmock.NewResult(0,1))
 mock.ExpectExec(regexp.QuoteMeta(upsertIncident)).WithArgs("error",sqlmock.AnyArg(),"unsolved","process-health/same-fault/"+current.Since.Format(time.RFC3339Nano),current.Since,current.LastNotified).WillReturnResult(sqlmock.NewResult(1,1))
 if err=Sync(context.Background(),db,path);err!=nil{t.Fatal(err)}
 if err=mock.ExpectationsWereMet();err!=nil{t.Fatal(err)}
}
