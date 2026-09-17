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
  const [apiKey, setApiKey] = useState<string>(() => sessionStorage.getItem('coldharbor_api_key') || '');
  const [workers, setWorkers] = useState<WorkerHeartbeat[]>([]);
  const [audits, setAudits] = useState<AuditRecord[]>([]);
  const [workerError, setWorkerError] = useState<string | null>(null);
  const [auditError, setAuditError] = useState<string | null>(null);
  const [selectedCompartmentId, setSelectedCompartmentId] = useState<string>('');
  const [isDispatchOpen, setIsDispatchOpen] = useState<boolean>(false);
  const [activeTab, setActiveTab] = useState<'overview' | 'telemetry' | 'tracker' | 'feed' | 'deaddrop' | 'audits'>('overview');

  const {
    loading: apiLoading,
    error: apiError,
    fetchWorkers,
    fetchAudits,
    fetchCompartment,
    fetchDeadDrop,
    dispatchJob,
  } = useApi(clearance, apiKey);

  const handleApiKeyChange = useCallback((key: string) => {
    setApiKey(key);
    if (key.trim()) {
      sessionStorage.setItem('coldharbor_api_key', key.trim());
    } else {
      sessionStorage.removeItem('coldharbor_api_key');
    }
  }, []);

  const {
    events,
    status: wsStatus,
    lastError: wsError,
    isPaused,
    togglePause,
    clearEvents,
    addEvent,
  } = useWebSocketEvents({ clearance, apiKey });

  // Load workers
  const loadWorkers = useCallback(async () => {
    try {
      const data = await fetchWorkers();
      setWorkers(data);
      setWorkerError(null);
    } catch (error) {
      setWorkers([]);
      setWorkerError(error instanceof Error ? error.message : 'Worker endpoint connection failed');
    }
  }, [fetchWorkers]);

  // Load audits
  const loadAudits = useCallback(async () => {
    try {
      const data = await fetchAudits();
      setAudits(data);
      setAuditError(null);
    } catch (error) {
      setAudits([]);
      setAuditError(error instanceof Error ? error.message : 'Audit endpoint connection failed');
    }
  }, [fetchAudits]);

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

  // Calculate active jobs
  const activeJobsCount = workers.filter(
    (w) => w.healthy && w.status === 'BUSY' && w.activeCompartmentId
  ).length;

  const handleSelectCompartment = (id: string) => {
    setSelectedCompartmentId(id);
    setActiveTab('tracker');
  };

  const closeDispatch = useCallback(() => setIsDispatchOpen(false), []);

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
        apiKey={apiKey}
        onApiKeyChange={handleApiKeyChange}
      />

      {/* Navigation Sub-bar */}
      <div className="border-b border-gray-800/80 bg-[#0d1322] px-6 py-2">
        <div className="max-w-7xl mx-auto flex items-center justify-between">
          <nav role="tablist" aria-label="Dashboard views" className="flex items-center gap-1 font-mono text-xs overflow-x-auto py-1">
            <button
              type="button"
              role="tab"
              aria-selected={activeTab === 'overview'}
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
              role="tab"
              aria-selected={activeTab === 'telemetry'}
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
              role="tab"
              aria-selected={activeTab === 'tracker'}
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
              role="tab"
              aria-selected={activeTab === 'feed'}
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
              role="tab"
              aria-selected={activeTab === 'deaddrop'}
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
              role="tab"
              aria-selected={activeTab === 'audits'}
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
        {(apiError || wsError) && (
          <div role="status" className="rounded-lg border border-rose-900/70 bg-rose-950/30 px-4 py-2 text-xs font-mono text-rose-300">
            {apiError && <div>API: {apiError}</div>}
            {wsError && <div>WebSocket: {wsError}</div>}
          </div>
        )}
        {/* Overview Tab: Displays all components in a master control room layout */}
        {activeTab === 'overview' && (
          <div className="space-y-6">
            {/* Top Row: Worker Telemetry */}
            <WorkerTelemetry
              workers={workers}
              loading={apiLoading}
              error={workerError}
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
                  fetchCompartment={fetchCompartment}
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
              error={auditError}
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
            error={workerError}
            onRefresh={loadWorkers}
            onSelectCompartment={handleSelectCompartment}
          />
        )}

        {activeTab === 'tracker' && (
          <LifecycleTracker
            selectedCompartmentId={selectedCompartmentId}
            onSelectCompartmentId={setSelectedCompartmentId}
            events={events}
            fetchCompartment={fetchCompartment}
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
            error={auditError}
            onRefresh={loadAudits}
            onSelectCompartment={handleSelectCompartment}
          />
        )}
      </main>

      {/* Modal Job Dispatcher */}
      <JobDispatcher
        isOpen={isDispatchOpen}
        onClose={closeDispatch}
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
