package docprocessing

import (
	"context"
	"errors"
	"fmt"
)

var errUnconfirmedRemoteDocument = errors.New("remote document requires confirmation")

func (s *ControlService) checkDocumentAdmission(ctx context.Context, recordID int64, allowRemoteDoc bool) error {
	if s.LeaseDB == nil {
		return nil
	}
	var origin string
	var syncing bool
	err := s.LeaseDB.QueryRowContext(ctx, `
SELECT i.doc_origin, EXISTS (SELECT 1 FROM kb.doc_sync_marks WHERE input_id = i.id)
FROM kb.inputs i WHERE i.id = $1`, recordID).Scan(&origin, &syncing)
	if err != nil {
		return fmt.Errorf("check document origin and sync mark for %d: %w", recordID, err)
	}
	if origin == "remote" && !allowRemoteDoc {
		err = errUnconfirmedRemoteDocument
	} else if syncing {
		err = ErrDocumentBusy
	}
	if err != nil && s.Logger != nil {
		s.Logger.Info("doc processor skipped work", "record_id", recordID, "reason", err)
	}
	return err
}

func (s *ControlService) skipUnconfirmedRemote(err error) error {
	if errors.Is(err, errUnconfirmedRemoteDocument) {
		return nil
	}
	return err
}

type documentStageGateKey struct{}

type documentStageGate struct {
	allowRemoteDoc bool
	stop           context.CancelCauseFunc
}

func (s *ControlService) admitDocumentStage(ctx context.Context, recordID int64) bool {
	gate, ok := ctx.Value(documentStageGateKey{}).(documentStageGate)
	if !ok {
		return true
	}
	if isCtxStopped(ctx) {
		return false
	}
	if err := s.checkDocumentAdmission(ctx, recordID, gate.allowRemoteDoc); err != nil {
		if s.Logger != nil {
			s.Logger.Warn("stopping pipeline before next processor", "record_id", recordID, "reason", err)
		}
		gate.stop(ErrPipelineStopped)
		return false
	}
	return true
}
