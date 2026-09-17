import React from 'react';
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/react';
import { Header } from '../components/Header';
import { WorkerTelemetry } from '../components/WorkerTelemetry';
import { LifecycleTracker } from '../components/LifecycleTracker';
import { DeadDropInspector } from '../components/DeadDropInspector';
import { AuditBrowser } from '../components/AuditBrowser';
import { EventFeed } from '../components/EventFeed';

describe('UI Component Rendering & Contracts', () => {
  it('renders Header with ColdHarbor title and clearance badges', () => {
    render(
      <Header
        clearance="INNIE"
        onClearanceChange={() => {}}
        wsStatus="CONNECTED"
        onOpenDispatch={() => {}}
        workerCount={2}
        activeJobsCount={1}
        totalEventsCount={5}
        onRefreshAll={() => {}}
      />
    );

    expect(screen.getByText('COLDHARBOR')).toBeInTheDocument();
    expect(screen.getByText('TELEMETRY v1.0')).toBeInTheDocument();
    expect(screen.getByText('CONNECTED')).toBeInTheDocument();
  });

  it('renders WorkerTelemetry with workers and health indicators', () => {
    const mockWorkers = [
      {
        workerId: 'worker-engine-01',
        status: 'IDLE' as const,
        activeCompartmentId: null,
        lastHeartbeat: new Date().toISOString(),
        healthy: true,
        secondsSinceLastHeartbeat: 0,
      },
      {
        workerId: 'worker-engine-02',
        status: 'BUSY' as const,
        activeCompartmentId: 'cpt_test_123',
        lastHeartbeat: new Date().toISOString(),
        healthy: true,
        secondsSinceLastHeartbeat: 0,
      },
      {
        workerId: 'worker-engine-03',
        status: 'DEAD' as const,
        activeCompartmentId: null,
        lastHeartbeat: new Date().toISOString(),
        healthy: false,
        secondsSinceLastHeartbeat: 45,
      },
    ];

    render(
      <WorkerTelemetry
        workers={mockWorkers}
        loading={false}
        onRefresh={() => {}}
        onSelectCompartment={() => {}}
      />
    );

    expect(screen.getByText('worker-engine-01')).toBeInTheDocument();
    expect(screen.getByText('worker-engine-02')).toBeInTheDocument();
    expect(screen.getByText('worker-engine-03')).toBeInTheDocument();
    expect(screen.getByText('DEAD / UNHEALTHY')).toBeInTheDocument();
    expect(screen.getByText('cpt_test_123')).toBeInTheDocument();
  });

  it('renders LifecycleTracker displaying FSM states', () => {
    render(
      <LifecycleTracker
        selectedCompartmentId="cpt_test_123"
        onSelectCompartmentId={() => {}}
        events={[
          {
            eventId: 'evt_1',
            compartmentId: 'cpt_test_123',
            workerId: 'worker-engine-01',
            fromState: 'CREATED',
            toState: 'RUNNING',
            checkpointPct: 25,
            timestamp: new Date().toISOString(),
          },
        ]}
        fetchCompartment={async () => ({
          compartmentId: 'cpt_test_123',
          context: 'INNIE',
          ownerId: 'usr_1',
          taskType: 'DATA_REDUCTION',
          state: 'RUNNING',
          progress: 25,
          createdAt: new Date().toISOString(),
        })}
      />
    );

    expect(screen.getAllByText('cpt_test_123').length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText('Created')).toBeInTheDocument();
    expect(screen.getByText('Queued')).toBeInTheDocument();
    expect(screen.getByText('Running')).toBeInTheDocument();
    expect(screen.getByText('Checkpoint')).toBeInTheDocument();
    expect(screen.getByText('Completed')).toBeInTheDocument();
    expect(screen.getByText('Archived')).toBeInTheDocument();
    expect(screen.getByText('Purged')).toBeInTheDocument();
  });

  it('renders DeadDropInspector lookup form', () => {
    render(
      <DeadDropInspector
        initialCompartmentId="cpt_sample"
        onFetchDeadDrop={async () => null}
      />
    );

    expect(screen.getByText('Dead Drop Archive Seal Inspector')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /inspect/i })).toBeInTheDocument();
  });

  it('renders AuditBrowser with audit records', () => {
    const mockAudits = [
      {
        id: 'uuid-1',
        compartmentId: 'cpt_audit_01',
        ownerId: 'usr_1',
        context: 'INNIE' as const,
        taskType: 'DATA_REDUCTION',
        finalState: 'PURGED' as const,
        checksum: 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
        durationMs: 500,
        createdAt: new Date().toISOString(),
        completedAt: new Date().toISOString(),
        metadata: {},
      },
    ];

    render(
      <AuditBrowser
        audits={mockAudits}
        loading={false}
        onRefresh={() => {}}
        onSelectCompartment={() => {}}
      />
    );

    expect(screen.getByText('PostgreSQL Immutable Audit Trail')).toBeInTheDocument();
    expect(screen.getByText('cpt_audit_01')).toBeInTheDocument();
    expect(screen.getByText('500ms')).toBeInTheDocument();
  });

  it('renders EventFeed live log console', () => {
    render(
      <EventFeed
        events={[
          {
            eventId: 'evt_1',
            compartmentId: 'cpt_feed_01',
            workerId: 'worker-engine-01',
            fromState: 'RUNNING',
            toState: 'CHECKPOINT',
            checkpointPct: 50,
            timestamp: new Date().toISOString(),
            details: 'Checkpoint 50% written',
          },
        ]}
        isPaused={false}
        onTogglePause={() => {}}
        onClear={() => {}}
        onSelectCompartment={() => {}}
      />
    );

    expect(screen.getByText('WebSocket Live Event Bus')).toBeInTheDocument();
    expect(screen.getByText('cpt_feed_01')).toBeInTheDocument();
    expect(screen.getByText('50%')).toBeInTheDocument();
  });
});
