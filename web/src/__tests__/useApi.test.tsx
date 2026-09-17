import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { useApi } from '../hooks/useApi';

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe('useApi request state', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('stays loading until all concurrent requests finish', async () => {
    const workers = deferred<any>();
    const audits = deferred<any>();
    vi.stubGlobal('fetch', vi.fn()
      .mockReturnValueOnce(workers.promise)
      .mockReturnValueOnce(audits.promise));

    const { result } = renderHook(() => useApi());
    let workersRequest!: Promise<unknown>;
    let auditsRequest!: Promise<unknown>;
    act(() => {
      workersRequest = result.current.fetchWorkers();
      auditsRequest = result.current.fetchAudits();
    });
    expect(result.current.loading).toBe(true);

    await act(async () => {
      workers.resolve({ ok: true, json: async () => [] });
      await workersRequest;
    });
    expect(result.current.loading).toBe(true);

    await act(async () => {
      audits.resolve({ ok: true, json: async () => [] });
      await auditsRequest;
    });
    expect(result.current.loading).toBe(false);
  });
});
