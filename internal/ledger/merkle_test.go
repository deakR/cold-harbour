package ledger

import (
	"bytes"
	"crypto/sha256"
	"testing"
)

func TestMerkleRootAndProof(t *testing.T) {
	// Create 4 leaf hashes
	leaf0 := sha256.Sum256([]byte("job_0"))
	leaf1 := sha256.Sum256([]byte("job_1"))
	leaf2 := sha256.Sum256([]byte("job_2"))
	leaf3 := sha256.Sum256([]byte("job_3"))

	leaves := [][]byte{leaf0[:], leaf1[:], leaf2[:], leaf3[:]}
	root := MerkleRoot(leaves)

	if len(root) != 32 {
		t.Fatalf("expected 32-byte root, got %d", len(root))
	}

	// Calculate expected intermediate nodes
	h01 := sha256.New()
	h01.Write(leaf0[:])
	h01.Write(leaf1[:])
	node01 := h01.Sum(nil)

	h23 := sha256.New()
	h23.Write(leaf2[:])
	h23.Write(leaf3[:])
	node23 := h23.Sum(nil)

	hRoot := sha256.New()
	hRoot.Write(node01)
	hRoot.Write(node23)
	expectedRoot := hRoot.Sum(nil)

	if !bytes.Equal(root, expectedRoot) {
		t.Fatalf("computed root does not match expected Merkle root")
	}

	// Verify proof for leaf0: sibling is leaf1 (on right), next sibling is node23 (on right)
	proof := [][]byte{leaf1[:], node23}
	isLeft := []bool{false, false}
	if !VerifyInclusionProof(leaf0[:], proof, isLeft, root) {
		t.Errorf("inclusion proof for leaf0 failed")
	}

	// Invalid proof should fail
	invalidProof := [][]byte{leaf2[:], node23}
	if VerifyInclusionProof(leaf0[:], invalidProof, isLeft, root) {
		t.Errorf("expected invalid proof to fail")
	}
}
