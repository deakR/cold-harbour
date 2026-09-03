import React, { useState } from 'react';
import {
  Terminal,
  Pause,
  Play,
  Trash2,
  Copy,
  Check,
  Filter,
  ArrowRight,
  Radio,
} from 'lucide-react';
import { EventMessage, normalizeEventMessage } from '../types';

interface EventFeedProps {
  events: EventMessage[];
  isPaused: boolean;
  onTogglePause: () => void;
  onClear: () => void;
  onSelectCompartment: (compartmentId: string) => void;
}

export const EventFeed: React.FC<EventFeedProps> = ({
  events,
  isPaused,
  onTogglePause,
  onClear,
  onSelectCompartment,
}) => {
  const [filterText, setFilterText] = useState<string>('');
  const [copiedId, setCopiedId] = useState<string | null>(null);

  const filteredEvents = events.filter((e) => {
    if (!filterText.trim()) return true;
    const q = filterText.toLowerCase();
    return (
      e.compartmentId?.toLowerCase().includes(q) ||
      e.workerId?.toLowerCase().includes(q) ||
      e.fromState?.toLowerCase().includes(q) ||
      e.toState?.toLowerCase().includes(q) ||
      e.details?.toLowerCase().includes(q)
    );
  });

  const handleCopy = (evt: EventMessage) => {
    navigator.clipboard.writeText(JSON.stringify(evt, null, 2));
    setCopiedId(evt.eventId);
    setTimeout(() => setCopiedId(null), 2000);
  };

  const getStateColor = (state: string) => {
    switch (state) {
      case 'CREATED':
      case 'QUEUED':
        return 'text-blue-400 bg-blue-950/40 border-blue-800';
      case 'RUNNING':
        return 'text-cyan-400 bg-cyan-950/40 border-cyan-800';
      case 'CHECKPOINT':
        return 'text-amber-400 bg-amber-950/40 border-amber-800';
      case 'COMPLETED':
      case 'ARCHIVED':
      case 'PURGED':
        return 'text-emerald-400 bg-emerald-950/40 border-emerald-800';
      case 'FAILED':
        return 'text-rose-400 bg-rose-950/40 border-rose-800';
      default:
        return 'text-gray-400 bg-gray-800 border-gray-700';
    }
  };

  return (
    <div className="bg-[#0f172a] rounded-xl border border-gray-800 shadow-xl overflow-hidden flex flex-col h-[520px]">
      {/* Feed Header */}
      <div className="p-4 bg-[#131d33] border-b border-gray-800 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-2.5">
          <Terminal className="w-5 h-5 text-emerald-400" />
          <h2 className="font-semibold text-white tracking-wide text-sm font-mono uppercase">
            WebSocket Live Event Bus
          </h2>
          <span className="text-xs px-2 py-0.5 rounded-full bg-gray-800 text-cyan-400 font-mono">
            {filteredEvents.length} {filteredEvents.length === 1 ? 'event' : 'events'}
          </span>
        </div>

        <div className="flex items-center gap-2">
          {/* Filter Input */}
          <div className="relative">
            <Filter className="w-3 h-3 text-gray-500 absolute left-2.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              placeholder="Filter events..."
              value={filterText}
              onChange={(e) => setFilterText(e.target.value)}
              className="bg-[#090d16] text-xs font-mono text-gray-200 pl-7 pr-2.5 py-1.5 rounded-lg border border-gray-700 focus:outline-none focus:border-cyan-500 w-36 sm:w-48"
            />
          </div>

          {/* Pause / Resume Button */}
          <button
            type="button"
            onClick={onTogglePause}
            className={`flex items-center gap-1 px-2.5 py-1.5 rounded-lg text-xs font-mono font-medium border transition ${
              isPaused
                ? 'bg-amber-950/60 text-amber-300 border-amber-800 hover:bg-amber-900/60'
                : 'bg-gray-800 text-gray-300 border-gray-700 hover:bg-gray-700'
            }`}
          >
            {isPaused ? <Play className="w-3.5 h-3.5" /> : <Pause className="w-3.5 h-3.5" />}
            <span>{isPaused ? 'RESUME' : 'PAUSE'}</span>
          </button>

          {/* Clear Log Button */}
          <button
            type="button"
            onClick={onClear}
            className="p-1.5 rounded-lg bg-gray-800 hover:bg-gray-700 text-gray-400 hover:text-gray-200 border border-gray-700 transition"
            title="Clear Event Log"
          >
            <Trash2 className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>

      {/* Events Terminal Body */}
      <div className="flex-1 p-3 overflow-y-auto font-mono text-xs space-y-2 bg-[#090d16]">
        {filteredEvents.length === 0 ? (
          <div className="h-full flex flex-col items-center justify-center text-gray-500 space-y-2">
            <Radio className="w-6 h-6 animate-pulse text-gray-600" />
            <p>Listening for real-time events on coldharbor:events (/ws/events)...</p>
          </div>
        ) : (
          filteredEvents.map((rawEvt) => {
            const evt = normalizeEventMessage(rawEvt);
            const isCopying = copiedId === evt.eventId;

            return (
              <div
                key={evt.eventId}
                className="p-2.5 rounded-lg bg-[#0e1626] border border-gray-800/80 hover:border-gray-700 transition flex flex-col sm:flex-row sm:items-center justify-between gap-2"
              >
                <div className="flex items-start sm:items-center gap-2.5">
                  <span className="text-gray-500 text-[11px] whitespace-nowrap">
                    {new Date(evt.timestamp).toLocaleTimeString()}
                  </span>

                  {/* Transition badges */}
                  <div className="flex items-center gap-1 text-[11px]">
                    <span className={`px-2 py-0.5 rounded border font-semibold ${getStateColor(evt.fromState)}`}>
                      {evt.fromState}
                    </span>
                    <ArrowRight className="w-3 h-3 text-gray-500" />
                    <span className={`px-2 py-0.5 rounded border font-semibold ${getStateColor(evt.toState)}`}>
                      {evt.toState}
                    </span>
                  </div>

                  {/* Checkpoint percentage tag */}
                  {evt.progress > 0 && (
                    <span className="px-1.5 py-0.5 rounded bg-amber-950 text-amber-400 border border-amber-800 text-[10px] font-bold">
                      {evt.progress}%
                    </span>
                  )}

                  {/* Compartment ID button */}
                  <button
                    type="button"
                    onClick={() => onSelectCompartment(evt.compartmentId)}
                    className="text-cyan-400 hover:text-cyan-300 hover:underline font-bold text-left truncate max-w-[180px]"
                    title="Click to view in FSM tracker"
                  >
                    {evt.compartmentId}
                  </button>
                </div>

                <div className="flex items-center justify-between sm:justify-end gap-3 text-gray-400 text-[11px]">
                  <span className="text-gray-400">{evt.workerId}</span>
                  {evt.details && (
                    <span className="text-gray-500 italic truncate max-w-[200px]" title={evt.details}>
                      {evt.details}
                    </span>
                  )}
                  <button
                    type="button"
                    onClick={() => handleCopy(rawEvt)}
                    className="p-1 text-gray-500 hover:text-gray-300 transition"
                    title="Copy event JSON"
                  >
                    {isCopying ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                  </button>
                </div>
              </div>
            );
          })
        )}
      </div>
    </div>
  );
};
