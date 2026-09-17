import React, { useEffect, useState } from 'react';
import {
  Layers,
  ArrowRight,
  CheckCircle2,
  Clock,
  AlertCircle,
  Database,
  Archive,
  Trash2,
  PlayCircle,
  GitBranch,
  Search,
} from 'lucide-react';
import { Compartment, CompartmentState, EventMessage, normalizeEventMessage } from '../types';

interface LifecycleTrackerProps {
  selectedCompartmentId: string;
  onSelectCompartmentId: (id: string) => void;
  events: EventMessage[];
  fetchCompartment: (id: string) => Promise<Compartment | null>;
}

const ORDERED_STEPS: { state: CompartmentState; label: string; icon: React.ElementType }[] = [
  { state: 'CREATED', label: 'Created', icon: PlusCircleIcon },
  { state: 'QUEUED', label: 'Queued', icon: Clock },
  { state: 'RUNNING', label: 'Running', icon: PlayCircle },
  { state: 'CHECKPOINT', label: 'Checkpoint', icon: GitBranch },
  { state: 'COMPLETED', label: 'Completed', icon: CheckCircle2 },
  { state: 'ARCHIVED', label: 'Archived', icon: Archive },
  { state: 'PURGED', label: 'Purged', icon: Trash2 },
];

function PlusCircleIcon(props: React.SVGProps<SVGSVGElement>) {
  return (
    <svg fill="none" viewBox="0 0 24 24" stroke="currentColor" strokeWidth={2} {...props}>
      <circle cx="12" cy="12" r="10" />
      <path strokeLinecap="round" strokeLinejoin="round" d="M12 8v8m-4-4h8" />
    </svg>
  );
}

export const LifecycleTracker: React.FC<LifecycleTrackerProps> = ({
  selectedCompartmentId,
  onSelectCompartmentId,
  events,
  fetchCompartment,
}) => {
  const [searchInput, setSearchInput] = useState<string>('');
  const [compartment, setCompartment] = useState<Compartment | null>(null);
  const [lookupError, setLookupError] = useState<string | null>(null);
  const [lookupLoading, setLookupLoading] = useState(false);

  useEffect(() => {
    if (!selectedCompartmentId) {
      setCompartment(null);
      setLookupError(null);
      return;
    }
    let active = true;
    setLookupLoading(true);
    setLookupError(null);
    fetchCompartment(selectedCompartmentId)
      .then((data) => {
        if (active) setCompartment(data);
      })
      .catch((error: unknown) => {
        if (!active) return;
        setCompartment(null);
        setLookupError(error instanceof Error ? error.message : 'Compartment lookup failed');
      })
      .finally(() => {
        if (active) setLookupLoading(false);
      });
    return () => {
      active = false;
    };
  }, [fetchCompartment, selectedCompartmentId]);

  // Extract events relevant to selected compartment
  const compartmentEvents = events
    .filter((e) => e.compartmentId && e.compartmentId.toLowerCase() === selectedCompartmentId.toLowerCase())
    .map(normalizeEventMessage);

  const latestEvent = compartmentEvents[0];
  const currentState = compartment?.state;
  const currentProgress = compartment?.progress ?? 0;
  const isFailed = currentState === 'FAILED';

  // List of distinct recent compartment IDs from all events for easy quick-switching
  const recentCompartments = Array.from(
    new Set(events.map((e) => e.compartmentId).filter(Boolean))
  ).slice(0, 6);

  const getStepIndex = (state: CompartmentState) => {
    return ORDERED_STEPS.findIndex((s) => s.state === state);
  };

  const currentStepIdx = currentState ? getStepIndex(currentState) : -1;

  const handleSearchSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (searchInput.trim()) {
      onSelectCompartmentId(searchInput.trim());
      setSearchInput('');
    }
  };

  return (
    <div className="bg-[#0f172a] rounded-xl border border-gray-800 shadow-xl overflow-hidden">
      {/* Header */}
      <div className="p-4 bg-[#131d33] border-b border-gray-800 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div className="flex items-center gap-2.5">
          <Layers className="w-5 h-5 text-indigo-400" />
          <h2 className="font-semibold text-white tracking-wide text-sm font-mono uppercase">
            Compartment Lifecycle FSM Tracker
          </h2>
        </div>

        {/* Compartment Search Form */}
        <form onSubmit={handleSearchSubmit} className="flex items-center gap-2">
          <div className="relative">
            <Search className="w-3.5 h-3.5 text-gray-500 absolute left-2.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              placeholder="Track Compartment ID..."
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              className="bg-[#090d16] text-xs font-mono text-gray-200 pl-8 pr-3 py-1.5 rounded-lg border border-gray-700 focus:outline-none focus:border-cyan-500 w-48 sm:w-60"
            />
          </div>
          <button
            type="submit"
            className="px-3 py-1.5 bg-indigo-600 hover:bg-indigo-500 text-white rounded-lg text-xs font-mono font-medium transition"
          >
            TRACK
          </button>
        </form>
      </div>

      <div className="p-5 space-y-6">
        {/* Quick select recent compartments */}
        {recentCompartments.length > 0 && (
          <div className="flex flex-wrap items-center gap-2 text-xs font-mono">
            <span className="text-gray-400">Recent:</span>
            {recentCompartments.map((id) => (
              <button
                key={id}
                type="button"
                onClick={() => onSelectCompartmentId(id)}
                className={`px-2.5 py-1 rounded border transition ${
                  selectedCompartmentId === id
                    ? 'bg-indigo-900/60 text-indigo-300 border-indigo-500 font-bold'
                    : 'bg-[#151f33] text-gray-400 border-gray-700 hover:border-gray-500 hover:text-gray-200'
                }`}
              >
                {id}
              </button>
            ))}
          </div>
        )}

        {/* Selected Compartment Header Banner */}
        <div className="bg-[#141e35] p-4 rounded-xl border border-gray-700/60 flex flex-col md:flex-row md:items-center justify-between gap-4">
          <div>
            <div className="flex items-center gap-2">
              <span className="text-xs text-gray-400 font-mono">TRACKING TARGET</span>
              {isFailed ? (
                <span className="px-2 py-0.5 rounded bg-rose-950 text-rose-400 border border-rose-800 text-[11px] font-mono font-bold flex items-center gap-1">
                  <AlertCircle className="w-3 h-3" /> FAILED
                </span>
              ) : (
                <span className="px-2 py-0.5 rounded bg-cyan-950 text-cyan-400 border border-cyan-800 text-[11px] font-mono font-bold">
                  {lookupLoading ? 'LOADING' : currentState || 'NOT FOUND'}
                </span>
              )}
            </div>
            <h3 className="text-lg font-bold font-mono text-white mt-1 break-all">
              {selectedCompartmentId || 'No compartment selected'}
            </h3>
          </div>

          <div className="flex items-center gap-4 text-xs font-mono">
            {latestEvent && (
              <div className="bg-[#090d16] px-3 py-2 rounded-lg border border-gray-800">
                <span className="text-gray-400 block text-[10px]">HANDLED BY</span>
                <span className="text-emerald-400 font-bold">{latestEvent.workerId}</span>
              </div>
            )}
            <div className="bg-[#090d16] px-3 py-2 rounded-lg border border-gray-800">
              <span className="text-gray-400 block text-[10px]">PROGRESS</span>
              <span className="text-amber-400 font-bold">{currentProgress}%</span>
            </div>
          </div>
        </div>

        {lookupError && (
          <div role="alert" className="p-3 rounded-lg bg-rose-950/40 border border-rose-800 text-xs font-mono text-rose-300">
            Server state unavailable: {lookupError}
          </div>
        )}

        {/* Stepper Pipeline Flow */}
        <div className="relative">
          <div className="grid grid-cols-2 sm:grid-cols-4 lg:grid-cols-7 gap-3">
            {ORDERED_STEPS.map((step, idx) => {
              const StepIcon = step.icon;
              const isPast = !isFailed && idx < currentStepIdx;
              const isCurrent = !isFailed && idx === currentStepIdx;
              const isFuture = !isFailed && (currentStepIdx < 0 || idx > currentStepIdx);

              let cardBg = 'bg-[#151f33] border-gray-800 text-gray-500';
              let iconColor = 'text-gray-600';

              if (isPast) {
                cardBg = 'bg-emerald-950/40 border-emerald-800/80 text-emerald-400';
                iconColor = 'text-emerald-400';
              } else if (isCurrent) {
                cardBg = 'bg-cyan-950/70 border-cyan-500 text-cyan-300 ring-2 ring-cyan-500/30';
                iconColor = 'text-cyan-400 animate-pulse';
              } else if (isFailed && step.state === currentState) {
                cardBg = 'bg-rose-950/70 border-rose-500 text-rose-300 ring-2 ring-rose-500/30';
                iconColor = 'text-rose-400';
              }

              return (
                <div
                  key={step.state}
                  className={`p-3 rounded-lg border transition-all flex flex-col items-center text-center relative ${cardBg}`}
                >
                  <div className="p-2 rounded-full bg-[#0b101c] mb-2">
                    <StepIcon className={`w-5 h-5 ${iconColor}`} />
                  </div>
                  <span className="text-xs font-mono font-bold">{step.label}</span>
                  <span className="text-[10px] font-mono mt-0.5 opacity-75">
                    {isCurrent ? (
                      <span className="text-cyan-400 font-semibold">ACTIVE</span>
                    ) : isPast ? (
                      <span className="text-emerald-400">DONE</span>
                    ) : (
                      'PENDING'
                    )}
                  </span>
                </div>
              );
            })}
          </div>

          {/* Progress bar */}
          <div className="mt-4 bg-gray-800/70 h-2 rounded-full overflow-hidden">
            <div
              className={`h-full transition-all duration-500 ${
                isFailed ? 'bg-rose-500' : 'bg-gradient-to-r from-cyan-500 via-indigo-500 to-emerald-400'
              }`}
              style={{
                width: isFailed
                  ? '100%'
                  : currentStepIdx < 0
                    ? '0%'
                    : `${Math.max(5, Math.min(100, (currentStepIdx / (ORDERED_STEPS.length - 1)) * 100))}%`,
              }}
            />
          </div>
        </div>

        {/* Transition History Table for this Compartment */}
        <div className="bg-[#0b101c] rounded-lg p-3 border border-gray-800">
          <div className="flex items-center justify-between mb-2">
            <span className="text-xs font-mono font-semibold text-gray-300">
              STATE TRANSITION AUDIT TRAIL ({compartmentEvents.length})
            </span>
          </div>

          {compartmentEvents.length === 0 ? (
            <p className="text-xs font-mono text-gray-500 py-3 text-center">
              No live transition events were captured in this browser session.
            </p>
          ) : (
            <div className="space-y-1.5 max-h-48 overflow-y-auto pr-1">
              {compartmentEvents.map((evt, i) => (
                <div
                  key={evt.eventId || i}
                  className="flex flex-col sm:flex-row sm:items-center justify-between text-xs font-mono p-2 rounded bg-[#131b2e] border border-gray-800 hover:border-gray-700 gap-1.5"
                >
                  <div className="flex items-center gap-2">
                    <span className="text-gray-400">{evt.fromState}</span>
                    <ArrowRight className="w-3.5 h-3.5 text-cyan-400" />
                    <span className="text-white font-bold">{evt.toState}</span>
                    {evt.progress > 0 && (
                      <span className="px-1.5 py-0.5 rounded bg-amber-950 text-amber-400 text-[10px] font-bold">
                        {evt.progress}%
                      </span>
                    )}
                  </div>
                  <div className="flex items-center gap-3 text-gray-400 text-[11px]">
                    <span className="text-cyan-400/80">{evt.workerId}</span>
                    <span>{new Date(evt.timestamp).toLocaleTimeString()}</span>
                  </div>
                </div>
              ))}
            </div>
          )}
          <p className="text-[10px] font-mono text-gray-600 mt-2">
            Current state is loaded from the server. Transition history contains only live events received during this session.
          </p>
        </div>
      </div>
    </div>
  );
};
