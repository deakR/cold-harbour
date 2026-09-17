import React from 'react';
import { Shield, Radio, Activity, PlusCircle, RefreshCw } from 'lucide-react';
import { ContextClearance } from '../types';
import { ConnectionStatus } from '../hooks/useWebSocketEvents';

interface HeaderProps {
  clearance: ContextClearance;
  onClearanceChange: (clearance: ContextClearance) => void;
  wsStatus: ConnectionStatus;
  onOpenDispatch: () => void;
  workerCount: number;
  activeJobsCount: number;
  totalEventsCount: number;
  onRefreshAll: () => void;
  apiKey?: string;
  onApiKeyChange?: (key: string) => void;
}

export const Header: React.FC<HeaderProps> = ({
  clearance,
  onClearanceChange,
  wsStatus,
  onOpenDispatch,
  workerCount,
  activeJobsCount,
  totalEventsCount,
  onRefreshAll,
  apiKey = '',
  onApiKeyChange,
}) => {
  const getStatusColor = (status: ConnectionStatus) => {
    switch (status) {
      case 'CONNECTED':
        return 'bg-emerald-500 text-emerald-300 border-emerald-500/30';
      case 'CONNECTING':
        return 'bg-amber-500 text-amber-300 border-amber-500/30';
      case 'ERROR':
      case 'DISCONNECTED':
        return 'bg-rose-500 text-rose-300 border-rose-500/30';
    }
  };

  const getClearanceBadgeColor = (c: ContextClearance) => {
    switch (c) {
      case 'INNIE':
        return 'border-indigo-500 text-indigo-400 bg-indigo-500/10';
      case 'OUTIE':
        return 'border-emerald-500 text-emerald-400 bg-emerald-500/10';
      case 'ADMIN':
        return 'border-purple-500 text-purple-400 bg-purple-500/10';
      case 'SYSTEM':
        return 'border-cyan-500 text-cyan-400 bg-cyan-500/10';
    }
  };

  return (
    <header className="border-b border-gray-800 bg-[#0c121e]/90 backdrop-blur sticky top-0 z-30 px-6 py-4">
      <div className="max-w-7xl mx-auto flex flex-col md:flex-row items-center justify-between gap-4">
        {/* Logo & Title */}
        <div className="flex items-center gap-3">
          <div className="p-2.5 rounded-xl bg-gradient-to-br from-cyan-500/20 to-indigo-500/20 border border-cyan-500/30 text-cyan-400">
            <Shield className="w-6 h-6" />
          </div>
          <div>
            <div className="flex items-center gap-2">
              <h1 className="text-xl font-bold tracking-tight text-white font-mono">
                COLDHARBOR
              </h1>
              <span className="text-xs px-2 py-0.5 rounded font-mono bg-cyan-950 text-cyan-400 border border-cyan-800">
                TELEMETRY v1.0
              </span>
            </div>
            <p className="text-xs text-gray-400">
              Distributed Compartmentalized Execution Engine
            </p>
          </div>
        </div>

        {/* Global Metrics Bar */}
        <div className="hidden lg:flex items-center gap-6 text-xs font-mono bg-[#070b12] px-4 py-2 rounded-lg border border-gray-800">
          <div className="flex items-center gap-2">
            <span className="w-2 h-2 rounded-full bg-emerald-400 animate-pulse"></span>
            <span className="text-gray-400">Workers:</span>
            <span className="font-semibold text-white">{workerCount}</span>
          </div>
          <div className="w-px h-4 bg-gray-800" />
          <div className="flex items-center gap-2">
            <Activity className="w-3.5 h-3.5 text-amber-400" />
            <span className="text-gray-400">Active Jobs:</span>
            <span className="font-semibold text-amber-400">{activeJobsCount}</span>
          </div>
          <div className="w-px h-4 bg-gray-800" />
          <div className="flex items-center gap-2">
            <Radio className="w-3.5 h-3.5 text-cyan-400" />
            <span className="text-gray-400">Live Events:</span>
            <span className="font-semibold text-cyan-400">{totalEventsCount}</span>
          </div>
        </div>

        {/* Controls & Clearance Switcher */}
        <div className="flex items-center gap-3">
          {/* WebSocket Status */}
          <div
            className={`flex items-center gap-1.5 px-2.5 py-1 rounded-full border text-xs font-mono font-medium ${getStatusColor(
              wsStatus
            )}`}
            title={`WebSocket: ${wsStatus}`}
          >
            <span
              className={`w-1.5 h-1.5 rounded-full ${
                wsStatus === 'CONNECTED' ? 'bg-emerald-400 animate-ping' : 'bg-current'
              }`}
            />
            {wsStatus}
          </div>

          {/* Context Clearance Dropdown */}
          <div className="flex items-center gap-1 bg-[#151c2c] border border-gray-700 rounded-lg p-1">
            <span className="text-xs text-gray-400 px-2 font-mono hidden sm:inline">CLEARANCE:</span>
            {(['INNIE', 'OUTIE', 'SYSTEM', 'ADMIN'] as ContextClearance[]).map((c) => (
              <button
                key={c}
                type="button"
                aria-pressed={clearance === c}
                onClick={() => onClearanceChange(c)}
                className={`px-2.5 py-1 text-xs font-mono font-medium rounded transition-all ${
                  clearance === c
                    ? `${getClearanceBadgeColor(c)} border shadow-sm`
                    : 'text-gray-400 hover:text-gray-200'
                }`}
              >
                {c}
              </button>
            ))}
          </div>

          {/* Refresh Button */}
          <button
            type="button"
            onClick={onRefreshAll}
            title="Refresh All Telemetry"
            className="p-2 rounded-lg bg-gray-800 hover:bg-gray-700 text-gray-300 transition-colors border border-gray-700"
          >
            <RefreshCw className="w-4 h-4" />
          </button>

          <div className="flex items-center">
            <input
              type="password"
              value={apiKey}
              onChange={(e) => onApiKeyChange?.(e.target.value)}
              placeholder="Session API key"
              aria-label="API key, stored for this browser session only"
              title="Sent as X-API-Key and cleared when this browser session ends"
              className="w-32 bg-[#151c2c] text-xs font-mono text-gray-200 px-2.5 py-1.5 rounded-l-lg border border-gray-700 focus:outline-none focus:border-cyan-500 placeholder:text-gray-600"
            />
            {apiKey && (
              <button
                type="button"
                onClick={() => onApiKeyChange?.('')}
                aria-label="Clear session API key"
                className="px-2 py-1.5 rounded-r-lg border border-l-0 border-gray-700 bg-gray-800 text-xs text-gray-400 hover:text-white"
              >
                ×
              </button>
            )}
          </div>

          {/* Dispatch Job Button */}
          <button
            type="button"
            onClick={onOpenDispatch}
            className="flex items-center gap-1.5 px-3.5 py-1.5 rounded-lg bg-cyan-600 hover:bg-cyan-500 text-white text-xs font-medium font-mono transition shadow-lg shadow-cyan-600/20"
          >
            <PlusCircle className="w-4 h-4" />
            <span>DISPATCH</span>
          </button>
        </div>
      </div>
    </header>
  );
};
