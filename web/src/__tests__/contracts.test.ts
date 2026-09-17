import { describe, it, expect } from 'vitest';
import {
  ALL_COMPARTMENT_STATES,
  normalizeAuditRecord,
  normalizeEventMessage,
  AuditRecord,
  EventMessage,
} from '../types';

describe('Contract Schema Normalization', () => {
  it('should include all 8 strict states defined in contracts.md', () => {
    expect(ALL_COMPARTMENT_STATES).toContain('CREATED');
    expect(ALL_COMPARTMENT_STATES).toContain('QUEUED');
    expect(ALL_COMPARTMENT_STATES).toContain('RUNNING');
    expect(ALL_COMPARTMENT_STATES).toContain('CHECKPOINT');
    expect(ALL_COMPARTMENT_STATES).toContain('COMPLETED');
    expect(ALL_COMPARTMENT_STATES).toContain('ARCHIVED');
    expect(ALL_COMPARTMENT_STATES).toContain('PURGED');
    expect(ALL_COMPARTMENT_STATES).toContain('FAILED');
  });

  it('should normalize camelCase and snake_case PostgreSQL audit records', () => {
    const rawSnake: AuditRecord = {
      id: 'uuid-1',
      compartment_id: 'cpt_snake_99',
      owner_id: 'usr_snake',
      context: 'INNIE',
      task_type: 'DATA_REDUCTION',
      final_state: 'PURGED',
      checksum: 'sha-abc',
      duration_ms: 320,
      created_at: '2026-08-18T10:00:00.000Z',
      completed_at: '2026-08-18T10:00:01.000Z',
      metadata: { items: 10 },
    };

    const normalized = normalizeAuditRecord(rawSnake);
    expect(normalized.compartmentId).toBe('cpt_snake_99');
    expect(normalized.ownerId).toBe('usr_snake');
    expect(normalized.taskType).toBe('DATA_REDUCTION');
    expect(normalized.finalState).toBe('PURGED');
    expect(normalized.durationMs).toBe(320);
    expect(normalized.checksum).toBe('sha-abc');
  });

  it('should normalize canonical and legacy event field names', () => {
    const rawContractEvent: EventMessage = {
      eventId: 'evt_1',
      compartmentId: 'cpt_1',
      workerId: 'worker-1',
      fromState: 'RUNNING',
      toState: 'CHECKPOINT',
      checkpointPct: 50,
      timestamp: '2026-08-18T10:00:05.120Z',
      details: 'Checkpoint 50% written',
    };

    const norm1 = normalizeEventMessage(rawContractEvent);
    expect(norm1.fromState).toBe('RUNNING');
    expect(norm1.toState).toBe('CHECKPOINT');
    expect(norm1.progress).toBe(50);

    const rawProjectEvent: EventMessage = {
      eventId: 'evt_2',
      compartmentId: 'cpt_2',
      workerId: 'worker-2',
      previousState: 'QUEUED',
      currentState: 'RUNNING',
      progress: 0,
      timestamp: '2026-08-18T10:00:06.000Z',
    };

    const norm2 = normalizeEventMessage(rawProjectEvent);
    expect(norm2.fromState).toBe('QUEUED');
    expect(norm2.toState).toBe('RUNNING');
    expect(norm2.progress).toBe(0);
  });
});
