export type ContextClearance = 'INNIE' | 'OUTIE' | 'SYSTEM' | 'ADMIN';

export type CompartmentState =
  | 'CREATED'
  | 'QUEUED'
  | 'RUNNING'
  | 'CHECKPOINT'
  | 'COMPLETED'
  | 'ARCHIVED'
  | 'PURGED'
  | 'FAILED';

export const ALL_COMPARTMENT_STATES: CompartmentState[] = [
  'CREATED',
  'QUEUED',
  'RUNNING',
  'CHECKPOINT',
  'COMPLETED',
  'ARCHIVED',
  'PURGED',
  'FAILED'
];

export type TaskType = 'DATA_REDUCTION' | 'CIPHER_STREAM' | 'ARCHIVE_SEAL' | string;

export interface JobDispatchPayload {
  compartmentId?: string;
  context: ContextClearance;
  ownerId: string;
  taskType: string;
  payload: Record<string, any> | string;
  maxRetries?: number;
  timeoutSeconds?: number;
}

export interface Compartment {
  id: string;
  context: ContextClearance;
  ownerId: string;
  taskType: string;
  currentState: CompartmentState;
  createdAt: string;
  updatedAt?: string;
  progress?: number;
}

export interface EventMessage {
  eventId: string;
  compartmentId: string;
  workerId: string;
  fromState?: CompartmentState;
  previousState?: CompartmentState;
  toState?: CompartmentState;
  currentState?: CompartmentState;
  checkpointPct?: number;
  progress?: number;
  timestamp: string;
  details?: string;
}

export interface WorkerHeartbeat {
  workerId: string;
  status: 'IDLE' | 'BUSY' | 'ACTIVE' | 'ALIVE' | 'DEAD' | 'UNHEALTHY';
  activeCompartmentId?: string | null;
  timestamp: string;
  lastSeenSeconds?: number;
}

export interface DeadDropResult {
  compartmentId: string;
  ownerId: string;
  context?: ContextClearance;
  taskType?: string;
  output?: Record<string, any> | string;
  resultPayload?: Record<string, any> | string;
  checksum: string;
  archivedAt?: string;
  completedAt?: string;
  ttlSeconds?: number;
  remainingTtlSeconds?: number;
  durationMs?: number;
}

export interface AuditRecord {
  id: string;
  compartmentId?: string;
  compartment_id?: string;
  ownerId?: string;
  owner_id?: string;
  context: ContextClearance;
  taskType?: string;
  task_type?: string;
  finalState?: CompartmentState;
  final_state?: CompartmentState;
  checksum: string;
  durationMs?: number;
  duration_ms?: number;
  createdAt?: string;
  created_at?: string;
  completedAt?: string;
  completed_at?: string;
  metadata?: Record<string, any>;
}

export interface NormalizedAuditRecord {
  id: string;
  compartmentId: string;
  ownerId: string;
  context: ContextClearance;
  taskType: string;
  finalState: CompartmentState;
  checksum: string;
  durationMs: number;
  createdAt: string;
  completedAt: string;
  metadata: Record<string, any>;
}

export function normalizeAuditRecord(raw: AuditRecord): NormalizedAuditRecord {
  return {
    id: raw.id,
    compartmentId: raw.compartmentId || raw.compartment_id || 'unknown',
    ownerId: raw.ownerId || raw.owner_id || 'system',
    context: raw.context,
    taskType: raw.taskType || raw.task_type || 'DATA_REDUCTION',
    finalState: (raw.finalState || raw.final_state || 'PURGED') as CompartmentState,
    checksum: raw.checksum || '',
    durationMs: raw.durationMs ?? raw.duration_ms ?? 0,
    createdAt: raw.createdAt || raw.created_at || new Date().toISOString(),
    completedAt: raw.completedAt || raw.completed_at || new Date().toISOString(),
    metadata: raw.metadata || {}
  };
}

export function normalizeEventMessage(raw: EventMessage): {
  eventId: string;
  compartmentId: string;
  workerId: string;
  fromState: CompartmentState;
  toState: CompartmentState;
  progress: number;
  timestamp: string;
  details: string;
} {
  const fromState = (raw.fromState || raw.previousState || 'CREATED') as CompartmentState;
  const toState = (raw.toState || raw.currentState || 'RUNNING') as CompartmentState;
  const progress = raw.progress ?? raw.checkpointPct ?? 0;
  return {
    eventId: raw.eventId || `evt_${Math.random().toString(36).substring(2, 9)}`,
    compartmentId: raw.compartmentId,
    workerId: raw.workerId || 'worker-system',
    fromState,
    toState,
    progress,
    timestamp: raw.timestamp || new Date().toISOString(),
    details: raw.details || `Transition ${fromState} -> ${toState}`
  };
}
