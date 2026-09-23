package ledger

import (
	"bytes"
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func Verify(ctx context.Context, conn *pgx.Conn, tenant uuid.UUID) error {
	rows, err := conn.Query(ctx, `
		SELECT seq, prev_hash, entry_hash, kind, payload, at
		FROM audit_ledger WHERE tenant_id = $1 ORDER BY seq
	`, tenant)
	if err != nil {
		return err
	}
	defer rows.Close()
	prev := Zeros()
	var expect int64 = 1
	var last []byte
	var n int64
	for rows.Next() {
		var seq int64
		var prevHash, entryHash []byte
		var kind string
		var payload []byte
		var at time.Time
		if err := rows.Scan(&seq, &prevHash, &entryHash, &kind, &payload, &at); err != nil {
			return err
		}
		if seq != expect {
			return fmt.Errorf("seq %d, want %d", seq, expect)
		}
		if !bytes.Equal(prevHash, prev) {
			return fmt.Errorf("seq %d prev_hash mismatch", seq)
		}
		sum := EntryHash(prev, kind, canonicalPayload(payload), normalizeAt(at))
		if !bytes.Equal(entryHash, sum) {
			return fmt.Errorf("seq %d entry_hash mismatch", seq)
		}
		prev = entryHash
		last = entryHash
		expect++
		n = seq
	}
	if err := rows.Err(); err != nil {
		return err
	}
	var headSeq int64
	var headHash []byte
	err = conn.QueryRow(ctx, `SELECT seq, hash FROM ledger_heads WHERE tenant_id = $1`, tenant).Scan(&headSeq, &headHash)
	if err != nil {
		if n == 0 {
			return nil
		}
		return err
	}
	if headSeq != n || !bytes.Equal(headHash, last) {
		return fmt.Errorf("head seq %d does not match chain", headSeq)
	}
	return nil
}
