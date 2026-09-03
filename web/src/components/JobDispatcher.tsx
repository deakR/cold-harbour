import React, { useState } from 'react';
import { Send, X, AlertCircle, CheckCircle2, Shield } from 'lucide-react';
import { ContextClearance, JobDispatchPayload } from '../types';

interface JobDispatcherProps {
  isOpen: boolean;
  onClose: () => void;
  currentClearance: ContextClearance;
  onDispatch: (payload: JobDispatchPayload) => Promise<{ compartmentId: string; status: string }>;
  onJobDispatched: (compartmentId: string) => void;
}

export const JobDispatcher: React.FC<JobDispatcherProps> = ({
  isOpen,
  onClose,
  currentClearance,
  onDispatch,
  onJobDispatched,
}) => {
  const [context, setContext] = useState<ContextClearance>(currentClearance);
  const [ownerId, setOwnerId] = useState<string>('usr_ops_01');
  const [taskType, setTaskType] = useState<string>('DATA_REDUCTION');
  const [payloadText, setPayloadText] = useState<string>(
    JSON.stringify({ batchSize: 500, inputValues: [10, 25, 42, 99, 150] }, null, 2)
  );
  const [submitting, setSubmitting] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [successId, setSuccessId] = useState<string | null>(null);

  if (!isOpen) return null;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setSuccessId(null);

    // Context clearance safety check: OUTIE users cannot dispatch INNIE compartments
    if (currentClearance === 'OUTIE' && context === 'INNIE') {
      setError('Context Clearance Violation: OUTIE clearance cannot dispatch INNIE workflows.');
      return;
    }

    let parsedPayload: any;
    try {
      parsedPayload = JSON.parse(payloadText);
    } catch (err: any) {
      setError(`Invalid JSON payload: ${err.message}`);
      return;
    }

    setSubmitting(true);
    try {
      const res = await onDispatch({
        context,
        ownerId,
        taskType,
        payload: parsedPayload,
      });

      setSuccessId(res.compartmentId);
      onJobDispatched(res.compartmentId);
    } catch (err: any) {
      setError(err.message || 'Dispatch failed');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70 backdrop-blur-sm">
      <div className="bg-[#0f172a] rounded-2xl border border-gray-700 shadow-2xl w-full max-w-lg overflow-hidden flex flex-col max-h-[90vh]">
        {/* Header */}
        <div className="p-4 bg-[#131d33] border-b border-gray-800 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Shield className="w-5 h-5 text-cyan-400" />
            <h3 className="font-semibold text-white font-mono text-sm tracking-wide">
              DISPATCH COMPARTMENT JOB
            </h3>
          </div>
          <button
            type="button"
            onClick={onClose}
            className="text-gray-400 hover:text-white p-1 rounded-lg hover:bg-gray-800 transition"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        {/* Form */}
        <form onSubmit={handleSubmit} className="p-5 space-y-4 overflow-y-auto font-mono text-xs">
          {error && (
            <div className="p-3 rounded-lg bg-rose-950/70 border border-rose-800 text-rose-300 flex items-start gap-2">
              <AlertCircle className="w-4 h-4 text-rose-400 shrink-0 mt-0.5" />
              <div>{error}</div>
            </div>
          )}

          {successId && (
            <div className="p-3 rounded-lg bg-emerald-950/70 border border-emerald-800 text-emerald-300 flex items-start gap-2">
              <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0 mt-0.5" />
              <div>
                <span className="font-bold">Compartment Dispatched!</span> ID: {successId}
              </div>
            </div>
          )}

          {/* Context clearance */}
          <div>
            <label className="block text-gray-400 mb-1 font-semibold">
              Context Clearance Level
            </label>
            <div className="grid grid-cols-2 gap-2">
              {(['INNIE', 'OUTIE'] as ContextClearance[]).map((c) => (
                <button
                  type="button"
                  key={c}
                  onClick={() => setContext(c)}
                  className={`py-2 px-3 rounded-lg border font-bold transition text-center ${
                    context === c
                      ? c === 'INNIE'
                        ? 'bg-indigo-950 text-indigo-300 border-indigo-500 ring-2 ring-indigo-500/30'
                        : 'bg-emerald-950 text-emerald-300 border-emerald-500 ring-2 ring-emerald-500/30'
                      : 'bg-[#151f33] text-gray-400 border-gray-700 hover:border-gray-600'
                  }`}
                >
                  {c}
                </button>
              ))}
            </div>
          </div>

          {/* Owner ID */}
          <div>
            <label className="block text-gray-400 mb-1 font-semibold">Owner ID</label>
            <input
              type="text"
              value={ownerId}
              onChange={(e) => setOwnerId(e.target.value)}
              className="w-full bg-[#090d16] text-gray-200 px-3 py-2 rounded-lg border border-gray-700 focus:outline-none focus:border-cyan-500"
              required
            />
          </div>

          {/* Task Type */}
          <div>
            <label className="block text-gray-400 mb-1 font-semibold">Task Type</label>
            <select
              value={taskType}
              onChange={(e) => setTaskType(e.target.value)}
              className="w-full bg-[#090d16] text-gray-200 px-3 py-2 rounded-lg border border-gray-700 focus:outline-none focus:border-cyan-500"
            >
              <option value="DATA_REDUCTION">DATA_REDUCTION (Map-Reduce & Sum)</option>
              <option value="CIPHER_STREAM">CIPHER_STREAM (Encryption & Tokenization)</option>
              <option value="ARCHIVE_SEAL">ARCHIVE_SEAL (SHA-256 Vaulting)</option>
            </select>
          </div>

          {/* Payload */}
          <div>
            <label className="block text-gray-400 mb-1 font-semibold">Job Payload (JSON)</label>
            <textarea
              rows={4}
              value={payloadText}
              onChange={(e) => setPayloadText(e.target.value)}
              className="w-full bg-[#090d16] text-gray-200 p-3 rounded-lg border border-gray-700 focus:outline-none focus:border-cyan-500 font-mono text-xs"
              required
            />
          </div>

          {/* Buttons */}
          <div className="flex items-center justify-end gap-3 pt-3 border-t border-gray-800">
            <button
              type="button"
              onClick={onClose}
              className="px-4 py-2 rounded-lg bg-gray-800 hover:bg-gray-700 text-gray-300 transition"
            >
              Cancel
            </button>
            <button
              type="submit"
              disabled={submitting}
              className="flex items-center gap-2 px-5 py-2 rounded-lg bg-cyan-600 hover:bg-cyan-500 text-white font-bold transition shadow-lg shadow-cyan-600/30 disabled:opacity-50"
            >
              <Send className="w-3.5 h-3.5" />
              <span>{submitting ? 'DISPATCHING...' : 'DISPATCH TO STREAM'}</span>
            </button>
          </div>
        </form>
      </div>
    </div>
  );
};
