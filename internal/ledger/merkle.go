package ledger

import (
	"bytes"
	"crypto/sha256"
)

// MerkleRoot calculates the cryptographic Merkle root of an array of leaf hashes.
func MerkleRoot(leaves [][]byte) []byte {
	if len(leaves) == 0 {
		return Zeros()
	}
	if len(leaves) == 1 {
		return leaves[0]
	}
	current := make([][]byte, len(leaves))
	copy(current, leaves)
	for len(current) > 1 {
		var next [][]byte
		for i := 0; i < len(current); i += 2 {
			if i+1 < len(current) {
				h := sha256.New()
				h.Write(current[i])
				h.Write(current[i+1])
				next = append(next, h.Sum(nil))
			} else {
				h := sha256.New()
				h.Write(current[i])
				h.Write(current[i])
				next = append(next, h.Sum(nil))
			}
		}
		current = next
	}
	return current[0]
}

// VerifyInclusionProof checks whether a specific leaf hash is cryptographically
// included in the Merkle root using a path of siblings.
// isLeft indicates whether the sibling is on the left side of the hash pair.
func VerifyInclusionProof(leaf []byte, siblings [][]byte, isLeft []bool, root []byte) bool {
	if len(siblings) != len(isLeft) {
		return false
	}
	current := leaf
	for i, sibling := range siblings {
		h := sha256.New()
		if isLeft[i] {
			h.Write(sibling)
			h.Write(current)
		} else {
			h.Write(current)
			h.Write(sibling)
		}
		current = h.Sum(nil)
	}
	return bytes.Equal(current, root)
}
