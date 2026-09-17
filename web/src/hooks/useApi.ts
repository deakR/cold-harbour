import { useState, useCallback, useRef } from 'react';
import {
  ContextClearance,
  WorkerHeartbeat,
  AuditRecord,
  DeadDropResult,
  Compartment,
  JobDispatchPayload
} from '../types';

const BASE_URL = '/api/v1';

export function useApi(currentClearance: ContextClearance = 'INNIE', apiKey: string = '') {
  const [loading, setLoading] = useState<boolean>(false);
  const [error, setError] = useState<string | null>(null);
  const activeRequestsRef = useRef(0);

  const beginRequest = useCallback(() => {
    activeRequestsRef.current += 1;
    setLoading(true);
  }, []);

  const endRequest = useCallback(() => {
    activeRequestsRef.current = Math.max(0, activeRequestsRef.current - 1);
    setLoading(activeRequestsRef.current > 0);
  }, []);

  const recordError = useCallback((err: unknown, fallback: string): Error => {
    const requestError = err instanceof Error ? err : new Error(fallback);
    setError(requestError.message || fallback);
    return requestError;
  }, []);

  const getHeaders = useCallback(() => {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
      'X-Context-Clearance': currentClearance,
      'X-ColdHarbor-Context': currentClearance,
    };
    if (apiKey.trim()) {
      headers['X-API-Key'] = apiKey.trim();
    }
    return headers;
  }, [currentClearance, apiKey]);

  const fetchWorkers = useCallback(async (): Promise<WorkerHeartbeat[]> => {
    beginRequest();
    try {
      const response = await fetch(`${BASE_URL}/workers`, {
        headers: getHeaders(),
      });
      if (!response.ok) {
        throw new Error(`Failed to fetch workers: HTTP ${response.status}`);
      }
      const data = await response.json();
      return Array.isArray(data) ? data : (data.workers || []);
    } catch (err: unknown) {
      throw recordError(err, 'Worker endpoint connection failed');
    } finally {
      endRequest();
    }
  }, [beginRequest, endRequest, getHeaders, recordError]);

  const fetchAudits = useCallback(
    async (params?: {
      compartmentId?: string;
      ownerId?: string;
      context?: string;
    }): Promise<AuditRecord[]> => {
      beginRequest();
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
      } catch (err: unknown) {
        throw recordError(err, 'Audit endpoint connection failed');
      } finally {
        endRequest();
      }
    },
    [beginRequest, endRequest, getHeaders, recordError]
  );

  const fetchCompartment = useCallback(
    async (id: string): Promise<Compartment | null> => {
      beginRequest();
      try {
        const response = await fetch(`${BASE_URL}/compartments/${encodeURIComponent(id)}`, {
          headers: getHeaders(),
        });
        if (!response.ok) {
          throw new Error(`Compartment ${id} not found: HTTP ${response.status}`);
        }
        const data = await response.json();
        return {
          ...data,
          compartmentId: data.compartmentId ?? data.id,
          state: data.state ?? data.currentState,
        };
      } catch (err: unknown) {
        throw recordError(err, 'Error fetching compartment');
      } finally {
        endRequest();
      }
    },
    [beginRequest, endRequest, getHeaders, recordError]
  );

  const fetchDeadDrop = useCallback(
    async (id: string): Promise<DeadDropResult | null> => {
      beginRequest();
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
      } catch (err: unknown) {
        throw recordError(err, 'Error fetching dead drop');
      } finally {
        endRequest();
      }
    },
    [beginRequest, endRequest, getHeaders, recordError]
  );

  const dispatchJob = useCallback(
    async (payload: JobDispatchPayload): Promise<{ compartmentId: string; status: string }> => {
      beginRequest();
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
      } catch (err: unknown) {
        throw recordError(err, 'Job dispatch failed');
      } finally {
        endRequest();
      }
    },
    [beginRequest, endRequest, getHeaders, recordError]
  );

  return {
    loading,
    error,
    clearError: () => setError(null),
    fetchWorkers,
    fetchAudits,
    fetchCompartment,
    fetchDeadDrop,
    dispatchJob,
  };
}
