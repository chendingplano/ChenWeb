package classfoundation

import (
	"context"
	"testing"
)

type fakeProvisionalClassCreator struct {
	created []ClassIdentity
}

func (f *fakeProvisionalClassCreator) CreateIdentityOnlyClass(_ context.Context, identity ClassIdentity) (ContractRevision, error) {
	f.created = append(f.created, identity)
	return ContractRevision{TermID: identity.TermID, DefinitionState: DefinitionIdentityOnly}, nil
}

func TestClassResolutionServiceUsesOnlyOneSafeCompatibleClass(t *testing.T) {
	creator := &fakeProvisionalClassCreator{}
	service := ClassResolutionService{ProvisionalClasses: creator}
	result, err := service.Resolve(context.Background(), ClassResolutionRequest{
		QuantityKindTermID: "quantity:temperature",
		Candidates: []ClassResolutionCandidate{
			{TermID: "metric:temperature", QuantityKindTermID: "quantity:temperature", Safe: true},
			{TermID: "metric:pressure", QuantityKindTermID: "quantity:pressure", Safe: true},
		},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.IdentityState != ResolutionResolvedExisting || result.SelectedClassTermID != "metric:temperature" {
		t.Fatalf("unexpected resolution: %#v", result)
	}
	if len(creator.created) != 0 {
		t.Fatalf("unexpected provisional class: %#v", creator.created)
	}
}

func TestClassResolutionServiceCreatesProvisionalClassWhenNoSafeCompatibleClass(t *testing.T) {
	creator := &fakeProvisionalClassCreator{}
	service := ClassResolutionService{ProvisionalClasses: creator}
	result, err := service.Resolve(context.Background(), ClassResolutionRequest{
		ProvisionalClass:   ClassIdentity{TermID: "metric:provisional-temperature", ModuleID: "metrics", By: "resolver"},
		QuantityKindTermID: "quantity:temperature",
		Candidates: []ClassResolutionCandidate{
			{TermID: "metric:temperature-label-only", Label: "Operating temperature", QuantityKindTermID: "quantity:pressure", Safe: true},
		},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.IdentityState != ResolutionProvisionalNew || result.SelectedClassTermID != "metric:provisional-temperature" {
		t.Fatalf("unexpected resolution: %#v", result)
	}
	if len(creator.created) != 1 || creator.created[0].TermID != "metric:provisional-temperature" {
		t.Fatalf("provisional creation = %#v", creator.created)
	}
}

func TestClassResolutionServiceMarksMultipleSafeCandidatesAmbiguousAndKeepsProvisionalClass(t *testing.T) {
	creator := &fakeProvisionalClassCreator{}
	result, err := (ClassResolutionService{ProvisionalClasses: creator}).Resolve(context.Background(), ClassResolutionRequest{
		ProvisionalClass:   ClassIdentity{TermID: "metric:provisional-speed", ModuleID: "metrics"},
		QuantityKindTermID: "quantity:speed",
		Candidates: []ClassResolutionCandidate{
			{TermID: "metric:normal-speed", QuantityKindTermID: "quantity:speed", Safe: true},
			{TermID: "metric:red-zone-speed", QuantityKindTermID: "quantity:speed", Safe: true},
		},
	})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if result.IdentityState != ResolutionAmbiguousCandidates || result.SelectedClassTermID != "metric:provisional-speed" {
		t.Fatalf("unexpected ambiguous resolution: %#v", result)
	}
}

func TestClassResolutionServiceRejectsClasslessOutcome(t *testing.T) {
	_, err := (ClassResolutionService{}).Resolve(context.Background(), ClassResolutionRequest{
		QuantityKindTermID: "quantity:temperature",
	})
	if err == nil {
		t.Fatal("expected unresolved source to be rejected instead of returned classless")
	}
}
