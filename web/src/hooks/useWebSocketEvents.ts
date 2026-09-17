import { useState, useEffect, useRef, useCallback } from 'react';
import { ContextClearance, EventMessage, normalizeEventMessage } from '../types';

export type ConnectionStatus = 'CONNECTING' | 'CONNECTED' | 'DISCONNECTED' | 'ERROR';

export interface UseWebSocketEventsOptions {
  url?: string;
  maxBuffer?: number;
  autoReconnect?: boolean;
  clearance?: ContextClearance;
  apiKey?: string;
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
  const reconnectTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pausedBufferRef = useRef<EventMessage[]>([]);
  const retryCountRef = useRef<number>(0);
  const pausedRef = useRef(false);
  const mountedRef = useRef(false);
  const shouldReconnectRef = useRef(autoReconnect);
  const connectRef = useRef<() => void>(() => {});

  const getWsUrl = useCallback(() => {
    if (options.url) return options.url;
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    // If running under Vite dev server proxy or standalone port 3000, target port 8080 or current host
    const host = window.location.port === '3000' ? `${window.location.hostname}:8080` : window.location.host;
    return `${protocol}//${host}/ws/events`;
  }, [options.url]);

  const getProtocols = useCallback(() => {
    const protocols = ['coldharbor', `clearance.${options.clearance ?? 'INNIE'}`];
    if (options.apiKey?.trim()) {
      const bytes = new TextEncoder().encode(options.apiKey.trim());
      let binary = '';
      bytes.forEach((byte) => {
        binary += String.fromCharCode(byte);
      });
      const encoded = btoa(binary).replace(/\+/g, '-').replace(/\//g, '_').replace(/=+$/, '');
      protocols.push(`api-key.${encoded}`);
    }
    return protocols;
  }, [options.clearance, options.apiKey]);

  const addEvent = useCallback((eventRaw: EventMessage) => {
    const normalized = normalizeEventMessage(eventRaw);
    if (pausedRef.current) {
      pausedBufferRef.current.push(normalized);
    } else {
      setEvents((prev) => [normalized, ...prev].slice(0, maxBuffer));
    }
  }, [maxBuffer]);

  const connect = useCallback(() => {
    shouldReconnectRef.current = autoReconnect;
    if (socketRef.current && (socketRef.current.readyState === WebSocket.OPEN || socketRef.current.readyState === WebSocket.CONNECTING)) {
      return;
    }

    const targetUrl = getWsUrl();
    setStatus('CONNECTING');
    setLastError(null);

    try {
      const ws = new WebSocket(targetUrl, getProtocols());
      socketRef.current = ws;

      ws.onopen = () => {
        if (socketRef.current !== ws) return;
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
        if (socketRef.current !== ws) return;
        setStatus('DISCONNECTED');
        socketRef.current = null;
        if (mountedRef.current && shouldReconnectRef.current) {
          const delay = Math.min(1000 * Math.pow(1.5, retryCountRef.current), 10000);
          retryCountRef.current += 1;
          reconnectTimeoutRef.current = setTimeout(() => {
            reconnectTimeoutRef.current = null;
            connectRef.current();
          }, delay);
        }
      };
    } catch (e: any) {
      setStatus('ERROR');
      setLastError(e.message || 'Failed to initialize WebSocket');
    }
  }, [getWsUrl, getProtocols, addEvent, autoReconnect]);

  const disconnect = useCallback(() => {
    shouldReconnectRef.current = false;
    if (reconnectTimeoutRef.current) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }
    const socket = socketRef.current;
    socketRef.current = null;
    if (socket) {
      socket.onclose = null;
      socket.close();
    }
    setStatus('DISCONNECTED');
  }, []);

  const togglePause = useCallback(() => {
    if (pausedRef.current) {
      pausedRef.current = false;
      setIsPaused(false);
      const buffered = pausedBufferRef.current.slice().reverse();
      pausedBufferRef.current = [];
      setEvents((current) => [...buffered, ...current].slice(0, maxBuffer));
      return;
    }
    pausedRef.current = true;
    setIsPaused(true);
  }, [maxBuffer]);

  const clearEvents = useCallback(() => {
    setEvents([]);
    pausedBufferRef.current = [];
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    shouldReconnectRef.current = autoReconnect;
    connectRef.current = connect;
    connect();
    return () => {
      mountedRef.current = false;
      disconnect();
    };
  }, [autoReconnect, connect, disconnect]);

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
