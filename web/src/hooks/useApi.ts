import { useState, useCallback } from 'react';
import {
  ContextClearance,
  WorkerHeartbeat,
  AuditRecord,
  DeadDropResult,
  Compartment,
  JobDispatchPayload
} from '../types';

const BASE_URL = '/api/v1';

export function useApi(currentClearance: ContextClearance = 'INNIE') {
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);

  const getHeaders = useCallback(() => {
    return {
      'Content-Type': 'application/json',
      'X-Context-Clearance': currentClearance,
      'X-ColdHarbor-Context': currentClearance,
    };
  }, [currentClearance]);

  const fetchWorkers = useCallback(async (): Promise<WorkerHeartbeat[]> => {
    setLoading(true);
    setError(null);
    try {
      const response = await fetch(`${BASE_URL}/workers`, {
        headers: getHeaders(),
      });
      if (!response.ok) {
        throw new Error(`Failed to fetch workers: HTTP ${response.status}`);
      }
      const data = await response.json();
      return Array.isArray(data) ? data : (data.workers || []);
    } catch (err: any) {
      const msg = err.message || 'Worker endpoint connection failed';
      setError(msg);
      return [];
    } finally {
      setLoading(false);
    }
  }, [getHeaders]);

  const fetchAudits = useCallback(
    async (params?: {
      compartmentId?: string;
      ownerId?: string;
      context?: string;
    }): Promise<AuditRecord[]> => {
      setLoading(true);
      setError(null);
      try {
        const query = new URLSearchParams();
        if (params?.compartmentId) query.set('compartmentId', params.compartmentId);
        if (params?.ownerId) query.set('ownerId', params.ownerId);
        if (params?.context && params.context !== 'ALL') query.set('context', params.context);

        const url = `${BASE_URL}/audits${query.toString() ? `?${query.toString()}` : ''}`;
        const response = await fetch(url, {
          headers: getHeaders(),
        });
        if (!response.ok) {
          throw new Error(`Failed to fetch audits: HTTP ${response.status}`);
        }
        const data = await response.json();
        return Array.isArray(data) ? data : (data.audits || data.content || []);
      } catch (err: any) {
        const msg = err.message || 'Audit endpoint connection failed';
        setError(msg);
        return [];
      } finally {
        setLoading(false);
      }
    },
    [getHeaders]
  );

  const fetchCompartment = useCallback(
    async (id: string): Promise<Compartment | null> => {
      setLoading(true);
      setError(null);
      try {
        const response = await fetch(`${BASE_URL}/compartments/${encodeURIComponent(id)}`, {
          headers: getHeaders(),
        });
        if (!response.ok) {
          throw new Error(`Compartment ${id} not found: HTTP ${response.status}`);
        }
        return await response.json();
      } catch (err: any) {
        setError(err.message || 'Error fetching compartment');
        return null;
      } finally {
        setLoading(false);
      }
    },
    [getHeaders]
  );

  const fetchDeadDrop = useCallback(
    async (id: string): Promise<DeadDropResult | null> => {
      setLoading(true);
      setError(null);
      try {
        const response = await fetch(
          `${BASE_URL}/compartments/${encodeURIComponent(id)}/deaddrop`,
          {
            headers: getHeaders(),
          }
        );
        if (!response.ok) {
          if (response.status === 404) {
            throw new Error(`Dead drop for ${id} not found (may not be sealed yet or expired)`);
          }
          throw new Error(`Dead drop retrieval failed: HTTP ${response.status}`);
        }
        return await response.json();
      } catch (err: any) {
        setError(err.message || 'Error fetching dead drop');
        throw err;
      } finally {
        setLoading(false);
      }
    },
    [getHeaders]
  );

  const dispatchJob = useCallback(
    async (payload: JobDispatchPayload): Promise<{ compartmentId: string; status: string }> => {
      setLoading(true);
      setError(null);
      try {
        const response = await fetch(`${BASE_URL}/compartments`, {
          method: 'POST',
          headers: getHeaders(),
          body: JSON.stringify(payload),
        });
        if (!response.ok) {
          const errText = await response.text();
          throw new Error(`Dispatch failed (HTTP ${response.status}): ${errText || response.statusText}`);
        }
        return await response.json();
      } catch (err: any) {
        const msg = err.message || 'Job dispatch failed';
        setError(msg);
        throw new Error(msg);
      } finally {
        setLoading(false);
      }
    },
    [getHeaders]
  );

  return {
    loading,
    error,
    fetchWorkers,
    fetchAudits,
    fetchCompartment,
    fetchDeadDrop,
    dispatchJob,
  };
}
