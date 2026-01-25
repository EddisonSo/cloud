import { useEffect, useRef, useState, useCallback } from "react";

export function useWebSocket(url, options = {}) {
  const { onMessage, onOpen, onClose, onError, enabled = true, reconnectDelay = 5000 } = options;
  const [connected, setConnected] = useState(false);
  const wsRef = useRef(null);
  const reconnectTimeoutRef = useRef(null);
  const isCleaningUpRef = useRef(false);

  const connect = useCallback(() => {
    if (isCleaningUpRef.current || !enabled || !url) return;

    wsRef.current = new WebSocket(url);

    wsRef.current.onopen = () => {
      setConnected(true);
      onOpen?.();
    };

    wsRef.current.onmessage = (event) => {
      onMessage?.(event);
    };

    wsRef.current.onerror = () => {
      onError?.();
    };

    wsRef.current.onclose = () => {
      setConnected(false);
      onClose?.();
      if (!isCleaningUpRef.current) {
        reconnectTimeoutRef.current = setTimeout(connect, reconnectDelay);
      }
    };
  }, [url, enabled, reconnectDelay, onMessage, onOpen, onClose, onError]);

  useEffect(() => {
    isCleaningUpRef.current = false;
    connect();

    return () => {
      isCleaningUpRef.current = true;
      if (reconnectTimeoutRef.current) {
        clearTimeout(reconnectTimeoutRef.current);
      }
      if (wsRef.current) {
        wsRef.current.close();
      }
    };
  }, [connect]);

  const send = useCallback((data) => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(typeof data === "string" ? data : JSON.stringify(data));
    }
  }, []);

  return { connected, send, ws: wsRef.current };
}
