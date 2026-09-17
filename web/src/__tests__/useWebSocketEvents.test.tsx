import { act, renderHook } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useWebSocketEvents } from '../hooks/useWebSocketEvents';

class MockWebSocket {
  static instances: MockWebSocket[] = [];
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSED = 3;

  readyState = MockWebSocket.CONNECTING;
  onopen: (() => void) | null = null;
  onmessage: ((event: { data: string }) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: (() => void) | null = null;

  constructor(public readonly url: string) {
    MockWebSocket.instances.push(this);
  }

  open() {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.();
  }

  sendJson(payload: unknown) {
    this.onmessage?.({ data: JSON.stringify(payload) });
  }

  close() {
    this.readyState = MockWebSocket.CLOSED;
    this.onclose?.();
  }
}

const event = (id: string) => ({
  eventId: id,
  compartmentId: 'cpt-1',
  workerId: 'worker-1',
  fromState: 'QUEUED' as const,
  toState: 'RUNNING' as const,
  timestamp: '2026-09-17T10:00:00Z',
});

describe('useWebSocketEvents lifecycle', () => {
  beforeEach(() => {
    MockWebSocket.instances = [];
    vi.stubGlobal('WebSocket', MockWebSocket);
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it('buffers while paused and resumes without reconnecting', () => {
    const { result } = renderHook(() => useWebSocketEvents({ url: 'ws://test/events' }));
    const socket = MockWebSocket.instances[0];

    act(() => socket.open());
    act(() => result.current.togglePause());
    act(() => socket.sendJson(event('evt-1')));
    expect(result.current.events).toEqual([]);

    act(() => result.current.togglePause());
    expect(result.current.events).toHaveLength(1);
    expect(result.current.events[0].eventId).toBe('evt-1');
    expect(MockWebSocket.instances).toHaveLength(1);
  });

  it('reconnects after an unexpected close but not after explicit disconnect', () => {
    vi.useFakeTimers();
    const { result, unmount } = renderHook(() =>
      useWebSocketEvents({ url: 'ws://test/events', autoReconnect: true })
    );

    act(() => MockWebSocket.instances[0].open());
    act(() => MockWebSocket.instances[0].close());
    act(() => vi.advanceTimersByTime(1000));
    expect(MockWebSocket.instances).toHaveLength(2);

    act(() => result.current.disconnect());
    act(() => vi.runAllTimers());
    expect(MockWebSocket.instances).toHaveLength(2);

    unmount();
    act(() => vi.runAllTimers());
    expect(MockWebSocket.instances).toHaveLength(2);
  });
});
