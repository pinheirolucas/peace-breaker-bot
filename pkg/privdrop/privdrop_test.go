package privdrop

import (
	"os"
	"testing"
)

func TestDropToNotRootIsNoOp(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("test must not run as root")
	}

	dropped, err := DropTo(t.TempDir(), 65532, 65532)
	if err != nil {
		t.Fatalf("DropTo returned error: %v", err)
	}
	if dropped {
		t.Fatal("DropTo reported privileges dropped while not running as root")
	}
}
