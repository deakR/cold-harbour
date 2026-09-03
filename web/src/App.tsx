import React, { useState, useEffect, useCallback } from 'react';
import {
  ContextClearance,
  WorkerHeartbeat,
  AuditRecord,
} from './types';
import { useApi } from './hooks/useApi';
import { useWebSocketEvents } from './hooks/useWebSocketEvents';
import { Header } from './components/Header';
import { WorkerTelemetry } from './components/WorkerTelemetry';
import { LifecycleTracker } from './components/LifecycleTracker';
import { EventFeed } from './components/EventFeed';
import { DeadDropInspector } from './components/DeadDropInspector';
import { AuditBrowser } from './components/AuditBrowser';
import { JobDispatcher } from './components/JobDispatcher';
import { Activity, Layers, Terminal, PackageCheck, FileSpreadsheet, Cpu } from 'lucide-react';

export const App: React.FC = () => {
  const [clearance, setClearance] = useState<ContextClearance>('INNIE');
  const [workers, setWorkers] = useState<WorkerHeartbeat[]>([]);
  const [audits, setAudits] = useState<AuditRecord[]>([]);
  const [selectedCompartmentId, setSelectedCompartmentId] = useState<string>('cpt_sample_01');
  const [isDispatchOpen, setIsDispatchOpen] = useState<boolean>(false);
  const [activeTab, setActiveTab] = useState<'overview' | 'telemetry' | 'tracker' | 'feed' | 'deaddrop' | 'audits'>('overview');

  const {
    loading: apiLoading,
    fetchWorkers,
    fetchAudits,
    fetchDeadDrop,
    dispatchJob,
  } = useApi(clearance);

  const {
    events,
    status: wsStatus,
    isPaused,
    togglePause,
    clearEvents,
    addEvent,
  } = useWebSocketEvents();

  // Load workers
  const loadWorkers = useCallback(async () => {
    const data = await fetchWorkers();
    if (data && data.length > 0) {
      setWorkers(data);
    } else if (workers.length === 0) {
      // Default sample workers for initial view if cluster is offline
      setWorkers([
        {
          workerId: 'worker-engine-01',
          status: 'IDLE',
          activeCompartmentId: null,
          timestamp: new Date().toISOString(),
        },
        {
          workerId: 'worker-engine-02',
          status: 'BUSY',
          activeCompartmentId: 'cpt_sample_01',
          timestamp: new Date().toISOString(),
        },
      ]);
    }
  }, [fetchWorkers, workers.length]);

  // Load audits
  const loadAudits = useCallback(async () => {
    const data = await fetchAudits();
    if (data && data.length > 0) {
      setAudits(data);
    } else if (audits.length === 0) {
      // Sample bootstrap audits if database empty
      setAudits([
        {
          id: '550e8400-e29b-41d4-a716-446655440000',
          compartmentId: 'cpt_sample_01',
          ownerId: 'usr_ops_01',
          context: 'INNIE',
          taskType: 'DATA_REDUCTION',
          finalState: 'PURGED',
          checksum: 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855',
          durationMs: 412,
          createdAt: new Date(Date.now() - 3600000).toISOString(),
          completedAt: new Date(Date.now() - 3599500).toISOString(),
          metadata: { processedCount: 500, checksumVerified: true },
        },
      ]);
    }
  }, [fetchAudits, audits.length]);

  const handleRefreshAll = useCallback(() => {
    loadWorkers();
    loadAudits();
  }, [loadWorkers, loadAudits]);

  // Initial load and polling intervals
  useEffect(() => {
    loadWorkers();
    loadAudits();

    const workerInterval = setInterval(loadWorkers, 8000);
    const auditInterval = setInterval(loadAudits, 12000);

    return () => {
      clearInterval(workerInterval);
      clearInterval(auditInterval);
    };
  }, [loadWorkers, loadAudits]);

  // If a new event arrives with a compartment ID, auto-select it if still on default
  useEffect(() => {
    if (events.length > 0 && selectedCompartmentId === 'cpt_sample_01') {
      const latest = events[0];
      if (latest.compartmentId) {
        setSelectedCompartmentId(latest.compartmentId);
      }
    }
  }, [events, selectedCompartmentId]);

  // Calculate active jobs
  const activeJobsCount = workers.filter((w) => w.status === 'BUSY' && w.activeCompartmentId).length;

  const handleSelectCompartment = (id: string) => {
    setSelectedCompartmentId(id);
    setActiveTab('tracker');
  };

  const handleJobDispatched = (id: string) => {
    setSelectedCompartmentId(id);
    setIsDispatchOpen(false);
    // Add local event to track immediately
    addEvent({
      eventId: `evt_${Date.now()}`,
      compartmentId: id,
      workerId: 'control-plane',
      fromState: 'CREATED',
      toState: 'QUEUED',
      checkpointPct: 0,
      timestamp: new Date().toISOString(),
      details: 'Job queued in coldharbor:jobs stream',
    });
  };

  return (
    <div className="min-h-screen bg-[#080c14] text-gray-100 flex flex-col font-sans">
      {/* Top Header */}
      <Header
        clearance={clearance}
        onClearanceChange={setClearance}
        wsStatus={wsStatus}
        onOpenDispatch={() => setIsDispatchOpen(true)}
        workerCount={workers.length}
        activeJobsCount={activeJobsCount}
        totalEventsCount={events.length}
        onRefreshAll={handleRefreshAll}
      />

      {/* Navigation Sub-bar */}
      <div className="border-b border-gray-800/80 bg-[#0d1322] px-6 py-2">
        <div className="max-w-7xl mx-auto flex items-center justify-between">
          <nav className="flex items-center gap-1 font-mono text-xs overflow-x-auto py-1">
            <button
              type="button"
              onClick={() => setActiveTab('overview')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg font-medium transition ${
                activeTab === 'overview'
                  ? 'bg-cyan-500/10 text-cyan-400 border border-cyan-500/30'
                  : 'text-gray-400 hover:text-gray-200'
              }`}
            >
              <Activity className="w-3.5 h-3.5" />
              <span>OVERVIEW</span>
            </button>

            <button
              type="button"
              onClick={() => setActiveTab('telemetry')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg font-medium transition ${
                activeTab === 'telemetry'
                  ? 'bg-cyan-500/10 text-cyan-400 border border-cyan-500/30'
                  : 'text-gray-400 hover:text-gray-200'
              }`}
            >
              <Cpu className="w-3.5 h-3.5" />
              <span>WORKERS</span>
            </button>

            <button
              type="button"
              onClick={() => setActiveTab('tracker')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg font-medium transition ${
                activeTab === 'tracker'
                  ? 'bg-indigo-500/10 text-indigo-400 border border-indigo-500/30'
                  : 'text-gray-400 hover:text-gray-200'
              }`}
            >
              <Layers className="w-3.5 h-3.5" />
              <span>FSM TRACKER</span>
            </button>

            <button
              type="button"
              onClick={() => setActiveTab('feed')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg font-medium transition ${
                activeTab === 'feed'
                  ? 'bg-emerald-500/10 text-emerald-400 border border-emerald-500/30'
                  : 'text-gray-400 hover:text-gray-200'
              }`}
            >
              <Terminal className="w-3.5 h-3.5" />
              <span>LIVE EVENTS</span>
            </button>

            <button
              type="button"
              onClick={() => setActiveTab('deaddrop')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg font-medium transition ${
                activeTab === 'deaddrop'
                  ? 'bg-amber-500/10 text-amber-400 border border-amber-500/30'
                  : 'text-gray-400 hover:text-gray-200'
              }`}
            >
              <PackageCheck className="w-3.5 h-3.5" />
              <span>DEAD DROP</span>
            </button>

            <button
              type="button"
              onClick={() => setActiveTab('audits')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-lg font-medium transition ${
                activeTab === 'audits'
                  ? 'bg-purple-500/10 text-purple-400 border border-purple-500/30'
                  : 'text-gray-400 hover:text-gray-200'
              }`}
            >
              <FileSpreadsheet className="w-3.5 h-3.5" />
              <span>AUDIT LOGS</span>
            </button>
          </nav>
        </div>
      </div>

      {/* Main Container */}
      <main className="flex-1 max-w-7xl w-full mx-auto p-4 sm:p-6 space-y-6">
        {/* Overview Tab: Displays all components in a master control room layout */}
        {activeTab === 'overview' && (
          <div className="space-y-6">
            {/* Top Row: Worker Telemetry */}
            <WorkerTelemetry
              workers={workers}
              loading={apiLoading}
              onRefresh={loadWorkers}
              onSelectCompartment={handleSelectCompartment}
            />

            {/* Middle Grid: FSM Tracker & Real-Time Event Feed */}
            <div className="grid grid-cols-1 lg:grid-cols-12 gap-6">
              <div className="lg:col-span-7">
                <LifecycleTracker
                  selectedCompartmentId={selectedCompartmentId}
                  onSelectCompartmentId={setSelectedCompartmentId}
                  events={events}
                />
              </div>
              <div className="lg:col-span-5">
                <EventFeed
                  events={events}
                  isPaused={isPaused}
                  onTogglePause={togglePause}
                  onClear={clearEvents}
                  onSelectCompartment={handleSelectCompartment}
                />
              </div>
            </div>

            {/* Bottom Row: Dead Drop Inspector */}
            <DeadDropInspector
              initialCompartmentId={selectedCompartmentId}
              onFetchDeadDrop={fetchDeadDrop}
            />

            {/* Historical Audit Trail */}
            <AuditBrowser
              audits={audits}
              loading={apiLoading}
              onRefresh={loadAudits}
              onSelectCompartment={handleSelectCompartment}
            />
          </div>
        )}

        {/* Dedicated Individual Views */}
        {activeTab === 'telemetry' && (
          <WorkerTelemetry
            workers={workers}
            loading={apiLoading}
            onRefresh={loadWorkers}
            onSelectCompartment={handleSelectCompartment}
          />
        )}

        {activeTab === 'tracker' && (
          <LifecycleTracker
            selectedCompartmentId={selectedCompartmentId}
            onSelectCompartmentId={setSelectedCompartmentId}
            events={events}
          />
        )}

        {activeTab === 'feed' && (
          <EventFeed
            events={events}
            isPaused={isPaused}
            onTogglePause={togglePause}
            onClear={clearEvents}
            onSelectCompartment={handleSelectCompartment}
          />
        )}

        {activeTab === 'deaddrop' && (
          <DeadDropInspector
            initialCompartmentId={selectedCompartmentId}
            onFetchDeadDrop={fetchDeadDrop}
          />
        )}

        {activeTab === 'audits' && (
          <AuditBrowser
            audits={audits}
            loading={apiLoading}
            onRefresh={loadAudits}
            onSelectCompartment={handleSelectCompartment}
          />
        )}
      </main>

      {/* Modal Job Dispatcher */}
      <JobDispatcher
        isOpen={isDispatchOpen}
        onClose={() => setIsDispatchOpen(false)}
        currentClearance={clearance}
        onDispatch={dispatchJob}
        onJobDispatched={handleJobDispatched}
      />

      {/* Footer */}
      <footer className="border-t border-gray-800/80 bg-[#090d16] px-6 py-4 mt-auto text-xs font-mono text-gray-500">
        <div className="max-w-7xl mx-auto flex flex-col sm:flex-row items-center justify-between gap-2">
          <div>
            COLDHARBOR TELEMETRY CONSOLE &copy; {new Date().getFullYear()} &mdash; ZERO-TRUST EXECUTION ENGINE
          </div>
          <div className="flex items-center gap-4 text-gray-400">
            <span>Context: {clearance}</span>
            <span>Redis Stream: coldharbor:jobs</span>
            <span>PubSub: coldharbor:events</span>
          </div>
        </div>
      </footer>
    </div>
  );
};
