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
	2: "605ccf3fff4a29d960decc645698a536333c9f47bc42d05f2fed0f9edbc3bc49",
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
