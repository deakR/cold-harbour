import React, { useState, useEffect } from 'react';
import { Cpu, Activity, Clock, Box, AlertTriangle, CheckCircle2, RefreshCw } from 'lucide-react';
import { WorkerHeartbeat } from '../types';

interface WorkerTelemetryProps {
  workers: WorkerHeartbeat[];
  loading: boolean;
  onRefresh: () => void;
  onSelectCompartment?: (compartmentId: string) => void;
}

export const WorkerTelemetry: React.FC<WorkerTelemetryProps> = ({
  workers,
  loading,
  onRefresh,
  onSelectCompartment,
}) => {
  const [currentTime, setCurrentTime] = useState<number>(Date.now());

  useEffect(() => {
    const timer = setInterval(() => setCurrentTime(Date.now()), 1000);
    return () => clearInterval(timer);
  }, []);

  const getWorkerLiveness = (worker: WorkerHeartbeat) => {
    const hbTime = new Date(worker.timestamp).getTime();
    const elapsedSeconds = Math.max(0, Math.floor((currentTime - hbTime) / 1000));
    
    // Status logic: if explicitly DEAD/UNHEALTHY or heartbeat > 30s
    if (worker.status === 'DEAD' || worker.status === 'UNHEALTHY' || elapsedSeconds > 30) {
      return {
        label: 'DEAD / UNHEALTHY',
        isAlive: false,
        badgeClass: 'bg-rose-950/80 text-rose-400 border-rose-800',
        dotClass: 'bg-rose-500',
        elapsedSeconds,
      };
    }

    if (worker.status === 'BUSY') {
      return {
        label: 'BUSY (EXECUTING)',
        isAlive: true,
        badgeClass: 'bg-amber-950/80 text-amber-400 border-amber-800',
        dotClass: 'bg-amber-400 animate-pulse',
        elapsedSeconds,
      };
    }

    return {
      label: 'IDLE (READY)',
      isAlive: true,
      badgeClass: 'bg-emerald-950/80 text-emerald-400 border-emerald-800',
      dotClass: 'bg-emerald-400',
      elapsedSeconds,
    };
  };

  return (
    <div className="bg-[#0f172a] rounded-xl border border-gray-800 shadow-xl overflow-hidden">
      {/* Header */}
      <div className="p-4 bg-[#131d33] border-b border-gray-800 flex items-center justify-between">
        <div className="flex items-center gap-2.5">
          <Cpu className="w-5 h-5 text-cyan-400" />
          <h2 className="font-semibold text-white tracking-wide text-sm font-mono uppercase">
            Worker Telemetry Cluster
          </h2>
          <span className="text-xs px-2 py-0.5 rounded-full bg-gray-800 text-gray-300 font-mono">
            {workers.length} {workers.length === 1 ? 'Node' : 'Nodes'}
          </span>
        </div>

        <button
          type="button"
          onClick={onRefresh}
          disabled={loading}
          className="flex items-center gap-1.5 text-xs text-cyan-400 hover:text-cyan-300 font-mono transition disabled:opacity-50"
        >
          <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          <span>POLL NOW</span>
        </button>
      </div>

      {/* Content */}
      <div className="p-4">
        {workers.length === 0 ? (
          <div className="text-center py-10 px-4 border border-dashed border-gray-800 rounded-lg">
            <Activity className="w-8 h-8 text-gray-600 mx-auto mb-2 animate-pulse" />
            <p className="text-sm font-mono text-gray-400">No active workers reporting heartbeats</p>
            <p className="text-xs text-gray-500 mt-1 max-w-sm mx-auto">
              Workers publish heartbeats to <code className="text-cyan-400 font-mono">worker:&#123;id&#125;:heartbeat</code> every 10s with 30s TTL.
            </p>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
            {workers.map((worker) => {
              const liveness = getWorkerLiveness(worker);

              return (
                <div
                  key={worker.workerId}
                  className="bg-[#17233d] rounded-lg p-4 border border-gray-700/60 hover:border-cyan-500/50 transition-all shadow-md flex flex-col justify-between space-y-3"
                >
                  {/* Worker ID & Status */}
                  <div className="flex items-start justify-between">
                    <div>
                      <div className="flex items-center gap-2">
                        <span className="text-xs text-gray-400 font-mono">WORKER ID</span>
                      </div>
                      <div className="text-base font-bold text-white font-mono tracking-tight mt-0.5">
                        {worker.workerId}
                      </div>
                    </div>

                    <div
                      className={`flex items-center gap-1.5 px-2 py-0.5 rounded border text-[11px] font-mono font-medium ${liveness.badgeClass}`}
                    >
                      <span className={`w-1.5 h-1.5 rounded-full ${liveness.dotClass}`} />
                      {liveness.label}
                    </div>
                  </div>

                  {/* Active Compartment */}
                  <div className="bg-[#0b101c] p-2.5 rounded border border-gray-800/80">
                    <div className="flex items-center justify-between text-xs font-mono text-gray-400 mb-1">
                      <span className="flex items-center gap-1">
                        <Box className="w-3.5 h-3.5 text-cyan-400" />
                        ACTIVE COMPARTMENT
                      </span>
                    </div>

                    {worker.activeCompartmentId ? (
                      <button
                        type="button"
                        onClick={() => onSelectCompartment && onSelectCompartment(worker.activeCompartmentId!)}
                        className="text-xs font-mono font-semibold text-cyan-300 hover:text-cyan-200 hover:underline truncate block w-full text-left"
                        title="Click to track lifecycle"
                      >
                        {worker.activeCompartmentId}
                      </button>
                    ) : (
                      <span className="text-xs font-mono text-gray-500 italic">None (Standby)</span>
                    )}
                  </div>

                  {/* Footer telemetry timestamp */}
                  <div className="flex items-center justify-between text-[11px] font-mono text-gray-400 pt-1 border-t border-gray-800">
                    <span className="flex items-center gap-1">
                      <Clock className="w-3 h-3 text-gray-500" />
                      Pulse: {liveness.elapsedSeconds}s ago
                    </span>

                    {liveness.isAlive ? (
                      <span className="flex items-center gap-1 text-emerald-400">
                        <CheckCircle2 className="w-3 h-3" />
                        Healthy
                      </span>
                    ) : (
                      <span className="flex items-center gap-1 text-rose-400 font-bold">
                        <AlertTriangle className="w-3 h-3" />
                        Expired &gt;30s
                      </span>
                    )}
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </div>
    </div>
  );
};
