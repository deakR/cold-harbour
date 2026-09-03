import React, { useState } from 'react';
import {
  FileSpreadsheet,
  Search,
  Filter,
  RefreshCw,
  ChevronDown,
  ChevronRight,
  Download,
  ShieldAlert,
  Clock,
} from 'lucide-react';
import { AuditRecord, normalizeAuditRecord, NormalizedAuditRecord } from '../types';

interface AuditBrowserProps {
  audits: AuditRecord[];
  loading: boolean;
  onRefresh: () => void;
  onSelectCompartment: (compartmentId: string) => void;
}

export const AuditBrowser: React.FC<AuditBrowserProps> = ({
  audits,
  loading,
  onRefresh,
  onSelectCompartment,
}) => {
  const [searchTerm, setSearchTerm] = useState<string>('');
  const [contextFilter, setContextFilter] = useState<string>('ALL');
  const [stateFilter, setStateFilter] = useState<string>('ALL');
  const [expandedId, setExpandedId] = useState<string | null>(null);

  const normalizedAudits: NormalizedAuditRecord[] = audits.map(normalizeAuditRecord);

  const filtered = normalizedAudits.filter((rec) => {
    if (contextFilter !== 'ALL' && rec.context !== contextFilter) return false;
    if (stateFilter !== 'ALL' && rec.finalState !== stateFilter) return false;
    if (searchTerm.trim()) {
      const q = searchTerm.toLowerCase();
      return (
        rec.compartmentId.toLowerCase().includes(q) ||
        rec.id.toLowerCase().includes(q) ||
        rec.ownerId.toLowerCase().includes(q) ||
        rec.taskType.toLowerCase().includes(q)
      );
    }
    return true;
  });

  const toggleExpand = (id: string) => {
    setExpandedId((prev) => (prev === id ? null : id));
  };

  const exportToJson = () => {
    const dataStr = 'data:text/json;charset=utf-8,' + encodeURIComponent(JSON.stringify(filtered, null, 2));
    const downloadAnchor = document.createElement('a');
    downloadAnchor.setAttribute('href', dataStr);
    downloadAnchor.setAttribute('download', `coldharbor_audit_records_${Date.now()}.json`);
    document.body.appendChild(downloadAnchor);
    downloadAnchor.click();
    downloadAnchor.remove();
  };

  const getContextColor = (ctx: string) => {
    switch (ctx) {
      case 'INNIE':
        return 'text-indigo-400 border-indigo-700 bg-indigo-950/40';
      case 'OUTIE':
        return 'text-emerald-400 border-emerald-700 bg-emerald-950/40';
      case 'ADMIN':
        return 'text-purple-400 border-purple-700 bg-purple-950/40';
      case 'SYSTEM':
        return 'text-cyan-400 border-cyan-700 bg-cyan-950/40';
      default:
        return 'text-gray-400 border-gray-700 bg-gray-800';
    }
  };

  const getStateColor = (st: string) => {
    switch (st) {
      case 'PURGED':
      case 'ARCHIVED':
      case 'COMPLETED':
        return 'text-emerald-400 border-emerald-800 bg-emerald-950/40';
      case 'FAILED':
        return 'text-rose-400 border-rose-800 bg-rose-950/40';
      default:
        return 'text-amber-400 border-amber-800 bg-amber-950/40';
    }
  };

  return (
    <div className="bg-[#0f172a] rounded-xl border border-gray-800 shadow-xl overflow-hidden">
      {/* Header */}
      <div className="p-4 bg-[#131d33] border-b border-gray-800 flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div className="flex items-center gap-2.5">
          <FileSpreadsheet className="w-5 h-5 text-indigo-400" />
          <h2 className="font-semibold text-white tracking-wide text-sm font-mono uppercase">
            PostgreSQL Immutable Audit Trail
          </h2>
          <span className="text-xs px-2 py-0.5 rounded-full bg-gray-800 text-gray-300 font-mono">
            {filtered.length} {filtered.length === 1 ? 'Record' : 'Records'}
          </span>
        </div>

        {/* Filter Controls */}
        <div className="flex flex-wrap items-center gap-2.5">
          {/* Search Input */}
          <div className="relative">
            <Search className="w-3.5 h-3.5 text-gray-500 absolute left-2.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              placeholder="Search compartment / UUID..."
              value={searchTerm}
              onChange={(e) => setSearchTerm(e.target.value)}
              className="bg-[#090d16] text-xs font-mono text-gray-200 pl-8 pr-3 py-1.5 rounded-lg border border-gray-700 focus:outline-none focus:border-cyan-500 w-44 sm:w-56"
            />
          </div>

          {/* Context Filter Dropdown */}
          <select
            value={contextFilter}
            onChange={(e) => setContextFilter(e.target.value)}
            className="bg-[#090d16] text-xs font-mono text-gray-300 px-2.5 py-1.5 rounded-lg border border-gray-700 focus:outline-none focus:border-cyan-500"
          >
            <option value="ALL">Context: ALL</option>
            <option value="INNIE">INNIE</option>
            <option value="OUTIE">OUTIE</option>
            <option value="SYSTEM">SYSTEM</option>
            <option value="ADMIN">ADMIN</option>
          </select>

          {/* State Filter Dropdown */}
          <select
            value={stateFilter}
            onChange={(e) => setStateFilter(e.target.value)}
            className="bg-[#090d16] text-xs font-mono text-gray-300 px-2.5 py-1.5 rounded-lg border border-gray-700 focus:outline-none focus:border-cyan-500"
          >
            <option value="ALL">State: ALL</option>
            <option value="PURGED">PURGED</option>
            <option value="ARCHIVED">ARCHIVED</option>
            <option value="COMPLETED">COMPLETED</option>
            <option value="FAILED">FAILED</option>
          </select>

          {/* Refresh & Export */}
          <button
            type="button"
            onClick={onRefresh}
            disabled={loading}
            className="p-2 rounded-lg bg-gray-800 hover:bg-gray-700 text-gray-300 border border-gray-700 transition"
            title="Refresh Audit Records"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />
          </button>

          <button
            type="button"
            onClick={exportToJson}
            disabled={filtered.length === 0}
            className="flex items-center gap-1.5 px-3 py-1.5 bg-gray-800 hover:bg-gray-700 disabled:opacity-50 text-gray-200 rounded-lg text-xs font-mono border border-gray-700 transition"
            title="Export to JSON"
          >
            <Download className="w-3.5 h-3.5" />
            <span className="hidden sm:inline">EXPORT</span>
          </button>
        </div>
      </div>

      {/* Table Body */}
      <div className="overflow-x-auto">
        <table className="w-full text-left font-mono text-xs text-gray-300">
          <thead className="bg-[#0b101c] text-gray-400 uppercase text-[10px] border-b border-gray-800">
            <tr>
              <th className="py-3 px-4 w-8"></th>
              <th className="py-3 px-4">Compartment ID</th>
              <th className="py-3 px-4">Context</th>
              <th className="py-3 px-4">Task Type</th>
              <th className="py-3 px-4">Final State</th>
              <th className="py-3 px-4">Duration</th>
              <th className="py-3 px-4">Checksum</th>
              <th className="py-3 px-4">Completed At</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-gray-800/60 bg-[#0f172a]">
            {filtered.length === 0 ? (
              <tr>
                <td colSpan={8} className="text-center py-10 text-gray-500">
                  {loading ? 'Loading audit records...' : 'No audit records matching query.'}
                </td>
              </tr>
            ) : (
              filtered.map((rec) => {
                const isExpanded = expandedId === rec.id;

                return (
                  <React.Fragment key={rec.id}>
                    <tr className="hover:bg-[#131d33] transition-colors">
                      <td className="py-3 px-4">
                        <button
                          type="button"
                          onClick={() => toggleExpand(rec.id)}
                          className="text-gray-400 hover:text-white"
                        >
                          {isExpanded ? (
                            <ChevronDown className="w-4 h-4 text-cyan-400" />
                          ) : (
                            <ChevronRight className="w-4 h-4" />
                          )}
                        </button>
                      </td>

                      <td className="py-3 px-4 font-bold">
                        <button
                          type="button"
                          onClick={() => onSelectCompartment(rec.compartmentId)}
                          className="text-cyan-400 hover:text-cyan-300 hover:underline text-left"
                          title="Click to track in FSM"
                        >
                          {rec.compartmentId}
                        </button>
                      </td>

                      <td className="py-3 px-4">
                        <span className={`px-2 py-0.5 rounded border text-[10px] font-bold ${getContextColor(rec.context)}`}>
                          {rec.context}
                        </span>
                      </td>

                      <td className="py-3 px-4 text-gray-300">{rec.taskType}</td>

                      <td className="py-3 px-4">
                        <span className={`px-2 py-0.5 rounded border text-[10px] font-bold ${getStateColor(rec.finalState)}`}>
                          {rec.finalState}
                        </span>
                      </td>

                      <td className="py-3 px-4 text-gray-300">
                        <span className="flex items-center gap-1">
                          <Clock className="w-3 h-3 text-gray-500" />
                          {rec.durationMs}ms
                        </span>
                      </td>

                      <td className="py-3 px-4">
                        <span className="text-amber-300/80 font-mono text-[11px]" title={rec.checksum}>
                          {rec.checksum ? `${rec.checksum.substring(0, 12)}...` : 'n/a'}
                        </span>
                      </td>

                      <td className="py-3 px-4 text-gray-400 text-[11px]">
                        {new Date(rec.completedAt).toLocaleString()}
                      </td>
                    </tr>

                    {isExpanded && (
                      <tr className="bg-[#090d16] border-b border-gray-800">
                        <td colSpan={8} className="p-4 space-y-3">
                          <div className="grid grid-cols-1 md:grid-cols-2 gap-4 text-xs">
                            <div>
                              <span className="text-gray-500 block">Record UUID:</span>
                              <span className="text-cyan-300">{rec.id}</span>
                            </div>
                            <div>
                              <span className="text-gray-500 block">Owner ID:</span>
                              <span className="text-gray-300">{rec.ownerId}</span>
                            </div>
                            <div>
                              <span className="text-gray-500 block">Full SHA-256 Checksum:</span>
                              <span className="text-amber-300 break-all">{rec.checksum}</span>
                            </div>
                            <div>
                              <span className="text-gray-500 block">Created At:</span>
                              <span className="text-gray-300">{new Date(rec.createdAt).toISOString()}</span>
                            </div>
                          </div>

                          <div>
                            <span className="text-gray-500 block mb-1">Execution Metadata (JSONB):</span>
                            <pre className="p-3 bg-[#0c1220] rounded border border-gray-800 text-[11px] text-gray-300 overflow-x-auto">
                              {JSON.stringify(rec.metadata, null, 2)}
                            </pre>
                          </div>
                        </td>
                      </tr>
                    )}
                  </React.Fragment>
                );
              })
            )}
          </tbody>
        </table>
      </div>
    </div>
  );
};
