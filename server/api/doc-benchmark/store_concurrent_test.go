package docbenchmark

import (
	"os"
	"testing"
)

// TestConcurrentClaim is reserved for the PostgreSQL integration harness. The
// harness must provide TEST_DATABASE_URL and an isolated benchmark fixture;
// without those prerequisites the sqlmock suite still exercises claim logic.
func TestConcurrentClaim(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL not set; PostgreSQL concurrent-claim prerequisite")
	}
	t.Fatalf("TEST_DATABASE_URL is set but isolated benchmark fixture/bootstrap is unavailable")
}
