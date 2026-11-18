package productreviews

import (
	"context"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestRelationWeightsOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("FROM kb.product_relation_types").
		WillReturnRows(sqlmock.NewRows([]string{"value", "weight"}).AddRow("pkg", 0.8).AddRow("scope", 0.9))
	s := DocumentScoper{DB: db, Config: &Config{}}
	s.Config.Scoring.RelationTypeWeights = map[string]float64{"scope": 1.0, "usage_condition": 0.6}
	w := s.relationWeights(context.Background())
	if w("pkg") != 0.8 || w("scope") != 0.9 {
		t.Fatalf("table weights not used: pkg=%v scope=%v", w("pkg"), w("scope"))
	}
	if w("usage_condition") != 0.6 {
		t.Fatalf("config fallback = %v, want 0.6", w("usage_condition"))
	}
	if w("unknown") != 0.3 {
		t.Fatalf("default = %v, want 0.3", w("unknown"))
	}
}

func TestRelationWeightsTableErrorFallsBack(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	mock.ExpectQuery("FROM kb.product_relation_types").WillReturnError(context.DeadlineExceeded)
	s := DocumentScoper{DB: db, Config: &Config{}}
	s.Config.Scoring.RelationTypeWeights = map[string]float64{"scope": 1.0}
	if got := s.relationWeights(context.Background())("scope"); got != 1.0 {
		t.Fatalf("got %v, want config 1.0", got)
	}
}
