export {};

declare global {
  interface Window {
    go: {
      main: {
        App: {
          GetConfig(): Promise<BSConfig>;
          SaveConfig(cfg: BSConfig): Promise<void>;
          DispatchBatch(taskType: string, values: number[]): Promise<string>;
          DispatchChaos(taskType: string, values: number[]): Promise<string>;
          BatchStatus(id: string): Promise<BSCompartment>;
          BatchReceipt(id: string): Promise<BSReceipt>;
          AuditHistory(owner: string): Promise<BSAudit[]>;
          LocalReceipts(): Promise<BSReceipt[]>;
          Fleet(): Promise<BSWorker[]>;
          DeadLetters(): Promise<number>;
          RedriveDLQ(limit: number): Promise<number>;
        };
      };
    };
  }

  interface BSConfig {
    baseURL: string;
    apiKey: string;
    clearance: string;
    ownerID: string;
  }

  interface BSCompartment {
    compartmentId: string;
    taskType: string;
    state: string;
    progress: number;
    details: string;
  }

  interface BSReceipt {
    compartmentId: string;
    taskType: string;
    output: Record<string, any>;
    checksum: string;
    verified: boolean;
    remainingTtlSeconds: number;
    archivedAt: string;
  }

  interface BSAudit {
    id: string;
    compartmentId: string;
    taskType: string;
    finalState: string;
    checksum: string;
    durationMs: number;
    completedAt: string;
  }

  interface BSWorker {
    workerId: string;
    status: string;
    healthy: boolean;
    activeCompartmentId: string;
    secondsSinceLastHeartbeat: number;
  }
}
