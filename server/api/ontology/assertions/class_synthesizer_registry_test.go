package assertions

import (
	"context"
	"testing"
)

func resetClassSynthesizerForTest(t *testing.T) {
	t.Helper()
	classSynthesizerMu.Lock()
	prev := defaultClassSynth
	defaultClassSynth = nil
	classSynthesizerMu.Unlock()
	t.Cleanup(func() {
		classSynthesizerMu.Lock()
		defaultClassSynth = prev
		classSynthesizerMu.Unlock()
	})
}

func TestRegisterClassSynthesizerRejectsSecondRegistration(t *testing.T) {
	resetClassSynthesizerForTest(t)
	fn := func(ctx context.Context, db DBX, candidateTermID string, input ClassSynthesisInput) (string, bool, error) {
		return candidateTermID, true, nil
	}
	if err := RegisterClassSynthesizer(fn); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	if err := RegisterClassSynthesizer(fn); err == nil {
		t.Fatal("expected error registering a second synthesizer, got nil")
	}
}

func TestRegisterClassSynthesizerRejectsNil(t *testing.T) {
	resetClassSynthesizerForTest(t)
	if err := RegisterClassSynthesizer(nil); err == nil {
		t.Fatal("expected error registering a nil synthesizer, got nil")
	}
}

func TestSynthesizeClassWithNoneRegisteredReturnsError(t *testing.T) {
	resetClassSynthesizerForTest(t)
	_, _, err := SynthesizeClass(context.Background(), nil, "measurement:x", ClassSynthesisInput{})
	if err == nil {
		t.Fatal("expected error when no synthesizer is registered, got nil")
	}
}

func TestSynthesizeClassDelegatesToRegistered(t *testing.T) {
	resetClassSynthesizerForTest(t)
	called := false
	if err := RegisterClassSynthesizer(func(ctx context.Context, db DBX, candidateTermID string, input ClassSynthesisInput) (string, bool, error) {
		called = true
		return candidateTermID, false, nil
	}); err != nil {
		t.Fatalf("register: %v", err)
	}
	termID, created, err := SynthesizeClass(context.Background(), nil, "measurement:x", ClassSynthesisInput{})
	if err != nil {
		t.Fatalf("SynthesizeClass: %v", err)
	}
	if !called {
		t.Fatal("expected registered synthesizer to be called")
	}
	if termID != "measurement:x" || created {
		t.Fatalf("unexpected result: termID=%q created=%v", termID, created)
	}
}
