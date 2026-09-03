import { useState, useEffect, useRef, useCallback } from 'react';
import { EventMessage, normalizeEventMessage } from '../types';

export type ConnectionStatus = 'CONNECTING' | 'CONNECTED' | 'DISCONNECTED' | 'ERROR';

export interface UseWebSocketEventsOptions {
  url?: string;
  maxBuffer?: number;
  autoReconnect?: boolean;
}

export function useWebSocketEvents(options: UseWebSocketEventsOptions = {}) {
  const {
    maxBuffer = 100,
    autoReconnect = true
  } = options;

  const [events, setEvents] = useState<EventMessage[]>([]);
  const [status, setStatus] = useState<ConnectionStatus>('DISCONNECTED');
  const [isPaused, setIsPaused] = useState<boolean>(false);
  const [lastError, setLastError] = useState<string | null>(null);

  const socketRef = useRef<WebSocket | null>(null);
  const reconnectTimeoutRef = useRef<any>(null);
  const pausedBufferRef = useRef<EventMessage[]>([]);
  const retryCountRef = useRef<number>(0);

  const getWsUrl = useCallback(() => {
    if (options.url) return options.url;
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    // If running under Vite dev server proxy or standalone port 3000, target port 8080 or current host
    const host = window.location.port === '3000' ? `${window.location.hostname}:8080` : window.location.host;
    return `${protocol}//${host}/ws/events`;
  }, [options.url]);

  const addEvent = useCallback((eventRaw: EventMessage) => {
    const normalized = normalizeEventMessage(eventRaw);
    if (isPaused) {
      pausedBufferRef.current.push(normalized);
    } else {
      setEvents((prev) => [normalized, ...prev].slice(0, maxBuffer));
    }
  }, [isPaused, maxBuffer]);

  const connect = useCallback(() => {
    if (socketRef.current && (socketRef.current.readyState === WebSocket.OPEN || socketRef.current.readyState === WebSocket.CONNECTING)) {
      return;
    }

    const targetUrl = getWsUrl();
    setStatus('CONNECTING');
    setLastError(null);

    try {
      const ws = new WebSocket(targetUrl);
      socketRef.current = ws;

      ws.onopen = () => {
        setStatus('CONNECTED');
        setLastError(null);
        retryCountRef.current = 0;
      };

      ws.onmessage = (messageEvent) => {
        try {
          const payload = JSON.parse(messageEvent.data);
          addEvent(payload);
        } catch (e) {
          console.warn('Failed to parse incoming WebSocket message:', messageEvent.data);
        }
      };

      ws.onerror = (_err) => {
        setStatus('ERROR');
        setLastError('WebSocket connection error');
      };

      ws.onclose = () => {
        setStatus('DISCONNECTED');
        socketRef.current = null;
        if (autoReconnect) {
          const delay = Math.min(1000 * Math.pow(1.5, retryCountRef.current), 10000);
          retryCountRef.current += 1;
          reconnectTimeoutRef.current = setTimeout(() => {
            connect();
          }, delay);
        }
      };
    } catch (e: any) {
      setStatus('ERROR');
      setLastError(e.message || 'Failed to initialize WebSocket');
    }
  }, [getWsUrl, addEvent, autoReconnect]);

  const disconnect = useCallback(() => {
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }
    if (socketRef.current) {
      socketRef.current.close();
      socketRef.current = null;
    }
    setStatus('DISCONNECTED');
  }, []);

  const togglePause = useCallback(() => {
    setIsPaused((prev) => {
      if (prev) {
        // Unpausing: flush buffered events
        setEvents((current) => [...pausedBufferRef.current, ...current].slice(0, maxBuffer));
        pausedBufferRef.current = [];
      }
      return !prev;
    });
  }, [maxBuffer]);

  const clearEvents = useCallback(() => {
    setEvents([]);
    pausedBufferRef.current = [];
  }, []);

  useEffect(() => {
    connect();
    return () => {
      disconnect();
    };
  }, [connect, disconnect]);

  return {
    events,
    status,
    isPaused,
    lastError,
    connect,
    disconnect,
    togglePause,
    clearEvents,
    addEvent, // Useful for testing & simulated transitions
  };
}
