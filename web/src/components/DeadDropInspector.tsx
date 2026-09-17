import React, { useState, useEffect } from 'react';
import {
  PackageCheck,
  Search,
  CheckCircle2,
  XCircle,
  Clock,
  KeyRound,
  Copy,
  Check,
  AlertCircle,
  FileCode,
  ShieldCheck,
} from 'lucide-react';
import { DeadDropResult } from '../types';
import { computeSha256, verifyChecksum } from '../utils/crypto';

interface DeadDropInspectorProps {
  initialCompartmentId?: string;
  onFetchDeadDrop: (compartmentId: string) => Promise<DeadDropResult | null>;
}

export const DeadDropInspector: React.FC<DeadDropInspectorProps> = ({
  initialCompartmentId = '',
  onFetchDeadDrop,
}) => {
  const [compartmentId, setCompartmentId] = useState<string>(initialCompartmentId);
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<DeadDropResult | null>(null);
  const [computedHash, setComputedHash] = useState<string | null>(null);
  const [isVerified, setIsVerified] = useState<boolean | null>(null);
  const [remainingTtl, setRemainingTtl] = useState<number | null>(null);
  const [copied, setCopied] = useState<boolean>(false);

  useEffect(() => {
    if (initialCompartmentId) {
      setCompartmentId(initialCompartmentId);
    }
  }, [initialCompartmentId]);

  // TTL decrement countdown timer
  useEffect(() => {
    if (remainingTtl === null || remainingTtl <= 0) return;
    const interval = setInterval(() => {
      setRemainingTtl((prev) => (prev !== null && prev > 0 ? prev - 1 : 0));
    }, 1000);
    return () => clearInterval(interval);
  }, [remainingTtl]);

  const handleInspect = async (e?: React.FormEvent) => {
    if (e) e.preventDefault();
    const id = compartmentId.trim();
    if (!id) return;

    setLoading(true);
    setError(null);
    setResult(null);
    setComputedHash(null);
    setIsVerified(null);

    try {
      const data = await onFetchDeadDrop(id);
      if (!data) {
        throw new Error('No dead drop payload returned for this compartment');
      }
      // Extract output or resultPayload
      const payloadToHash = data.output ?? data.resultPayload ?? {};
      const calculated = await computeSha256(payloadToHash);
      setResult(data);
      setComputedHash(calculated);

      // Verify checksum
      const matched = verifyChecksum(data.checksum, calculated);
      setIsVerified(matched);

      // Initialize TTL from server-reported remaining time when available.
      // remainingTtlSeconds === 0 means Redis TTL expired and the payload was
      // served from the durable PostgreSQL audit record.
      if (typeof data.remainingTtlSeconds === 'number') {
        setRemainingTtl(data.remainingTtlSeconds);
      } else if (typeof data.ttlSeconds === 'number') {
        setRemainingTtl(data.ttlSeconds);
      } else {
        setRemainingTtl(3600); // default
      }
    } catch (err: any) {
      setError(err.message || 'Failed to inspect dead drop archive');
    } finally {
      setLoading(false);
    }
  };

  const handleCopy = () => {
    if (!result) return;
    navigator.clipboard.writeText(JSON.stringify(result, null, 2));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="bg-[#0f172a] rounded-xl border border-gray-800 shadow-xl overflow-hidden">
      {/* Header */}
      <div className="p-4 bg-[#131d33] border-b border-gray-800 flex flex-col sm:flex-row sm:items-center justify-between gap-3">
        <div className="flex items-center gap-2.5">
          <PackageCheck className="w-5 h-5 text-cyan-400" />
          <h2 className="font-semibold text-white tracking-wide text-sm font-mono uppercase">
            Dead Drop Archive Seal Inspector
          </h2>
        </div>

        {/* Lookup Bar */}
        <form onSubmit={handleInspect} className="flex items-center gap-2">
          <div className="relative">
            <Search className="w-3.5 h-3.5 text-gray-500 absolute left-2.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              placeholder="Compartment ID..."
              value={compartmentId}
              onChange={(e) => setCompartmentId(e.target.value)}
              className="bg-[#090d16] text-xs font-mono text-gray-200 pl-8 pr-3 py-1.5 rounded-lg border border-gray-700 focus:outline-none focus:border-cyan-500 w-48 sm:w-64"
            />
          </div>
          <button
            type="submit"
            disabled={loading || !compartmentId.trim()}
            className="px-3.5 py-1.5 bg-cyan-600 hover:bg-cyan-500 disabled:opacity-50 text-white rounded-lg text-xs font-mono font-medium transition"
          >
            {loading ? 'VERIFYING...' : 'INSPECT'}
          </button>
        </form>
      </div>

      {/* Body Content */}
      <div className="p-5 space-y-4">
        {error && (
          <div className="p-3.5 rounded-lg bg-rose-950/60 border border-rose-800 flex items-start gap-2.5 text-xs text-rose-300 font-mono">
            <AlertCircle className="w-4 h-4 text-rose-400 shrink-0 mt-0.5" />
            <div>
              <span className="font-bold">Inspection Failed:</span> {error}
            </div>
          </div>
        )}

        {!result && !loading && !error && (
          <div className="text-center py-10 px-4 border border-dashed border-gray-800 rounded-lg">
            <ShieldCheck className="w-8 h-8 text-gray-600 mx-auto mb-2" />
            <p className="text-sm font-mono text-gray-400">
              Enter a completed compartment ID to inspect its cryptographic Dead Drop
            </p>
            <p className="text-xs text-gray-500 mt-1">
              Validates SHA-256 seal integrity and active Redis expiration TTL (<code className="text-cyan-400">archive:&#123;id&#125;</code>)
            </p>
          </div>
        )}

        {result && (
          <div className="space-y-4 font-mono text-xs">
            {/* Top Status Banner */}
            <div className="grid grid-cols-1 md:grid-cols-3 gap-3">
              {/* SHA-256 Verification Badge */}
              <div
                className={`p-3 rounded-lg border flex items-center justify-between ${
                  isVerified
                    ? 'bg-emerald-950/50 border-emerald-700 text-emerald-300'
                    : 'bg-rose-950/50 border-rose-700 text-rose-300'
                }`}
              >
                <div>
                  <span className="text-[10px] text-gray-400 block">SEAL INTEGRITY</span>
                  <span className="font-bold text-sm">
                    {isVerified ? 'CRYPTOGRAPHICALLY SEALED' : 'SEAL MISMATCH'}
                  </span>
                </div>
                {isVerified ? (
                  <CheckCircle2 className="w-6 h-6 text-emerald-400" />
                ) : (
                  <XCircle className="w-6 h-6 text-rose-400" />
                )}
              </div>

              {/* TTL Countdown */}
              <div className="p-3 rounded-lg bg-[#141d33] border border-gray-700 flex items-center justify-between">
                <div>
                  <span className="text-[10px] text-gray-400 block">TIME TO LIVE (TTL)</span>
                  <span className="font-bold text-sm text-cyan-400">
                    {remainingTtl !== null && remainingTtl > 0
                      ? `${remainingTtl}s remaining`
                      : 'EXPIRED — SERVED FROM DURABLE AUDIT'}
                  </span>
                </div>
                <Clock className="w-6 h-6 text-cyan-400" />
              </div>

              {/* Context Clearance & Owner */}
              <div className="p-3 rounded-lg bg-[#141d33] border border-gray-700 flex items-center justify-between">
                <div>
                  <span className="text-[10px] text-gray-400 block">CONTEXT CLEARANCE</span>
                  <span className="font-bold text-sm text-indigo-400">
                    {result.context || 'INNIE'}
                  </span>
                </div>
                <div className="text-right">
                  <span className="text-[10px] text-gray-400 block">OWNER</span>
                  <span className="text-gray-300">{result.ownerId || 'system'}</span>
                </div>
              </div>
            </div>

            {/* Checksum Details */}
            <div className="bg-[#090d16] p-3.5 rounded-lg border border-gray-800 space-y-2">
              <div className="flex items-center gap-1.5 text-gray-300 font-semibold text-[11px]">
                <KeyRound className="w-3.5 h-3.5 text-amber-400" />
                <span>SHA-256 HASH VERIFICATION</span>
              </div>

              <div className="space-y-1 text-[11px]">
                <div>
                  <span className="text-gray-500">Claimed Checksum: </span>
                  <span className="text-amber-300 break-all">{result.checksum}</span>
                </div>
                {computedHash && (
                  <div>
                    <span className="text-gray-500">Computed Hash:   </span>
                    <span className={isVerified ? 'text-emerald-400 break-all' : 'text-rose-400 break-all'}>
                      {computedHash}
                    </span>
                  </div>
                )}
              </div>
            </div>

            {/* Output Payload Viewer */}
            <div className="bg-[#090d16] rounded-lg border border-gray-800 overflow-hidden">
              <div className="p-2.5 bg-[#101726] border-b border-gray-800 flex items-center justify-between text-gray-400 text-[11px]">
                <div className="flex items-center gap-1.5">
                  <FileCode className="w-3.5 h-3.5 text-cyan-400" />
                  <span className="font-semibold text-gray-300">SEALED OUTPUT PAYLOAD</span>
                </div>
                <button
                  type="button"
                  onClick={handleCopy}
                  className="flex items-center gap-1 text-gray-400 hover:text-gray-200 transition"
                >
                  {copied ? <Check className="w-3.5 h-3.5 text-emerald-400" /> : <Copy className="w-3.5 h-3.5" />}
                  <span>{copied ? 'COPIED' : 'COPY RAW JSON'}</span>
                </button>
              </div>

              <pre className="p-4 text-gray-300 overflow-x-auto text-xs max-h-60">
                {JSON.stringify(result.output ?? result.resultPayload ?? result, null, 2)}
              </pre>
            </div>
          </div>
        )}
      </div>
    </div>
  );
};
