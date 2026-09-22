package piplayer

import (
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// TestMain lowers the bcrypt work factor for the whole package. At the
// production cost the suite spends about 92% of its time hashing, and roughly
// twelve times that under -race. Setting it here rather than per-test keeps it
// off the hot path of any future parallel test.
func TestMain(m *testing.M) {
	hashCost = bcrypt.MinCost
	// Otherwise a websocket test's "send buffer full" chatter buries whatever
	// actually failed.
	discardLogs()
	m.Run()
}

// TestProductionHashCost makes sure lowering the cost for tests can't quietly
// lower it in production.
func TestProductionHashCost(t *testing.T) {
	if productionHashCost < 12 {
		t.Errorf("productionHashCost = %d, want at least 12", productionHashCost)
	}
}
