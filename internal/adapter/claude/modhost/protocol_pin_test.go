package modhost

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

// protocolDigests pins each mod contract to the generated protocol it names:
// a loaded mod keeps speaking the protocol it was loaded with, so a change of
// it raises Protocol, and a switch of the room's build refuses the new one.
var protocolDigests = map[int]string{
	1: "17a3817dd7671b50f647a861cfed64fa3e7bccbf348818fb8d9c6308022b6642",
	2: "ac975e0bbd602b27960db0ab901cee36f578a7c31ee0e72d756aebb1132ed440",
}

func TestTheModContractChangesWithItsProtocol(t *testing.T) {
	data, err := os.ReadFile("protocol_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	if pinned, ok := protocolDigests[Protocol]; !ok || pinned != digest {
		t.Fatalf("protocol_gen.go is %s, and mod contract %d pins %q: a changed protocol raises Protocol and pins its digest", digest, Protocol, pinned)
	}
}
