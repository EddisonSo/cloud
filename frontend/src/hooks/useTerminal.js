import { useState, useRef, useEffect, useCallback } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";

export function useTerminal() {
  const [container, setContainer] = useState(null);
  const [connecting, setConnecting] = useState(false);
  const [error, setError] = useState("");
  const terminalRef = useRef(null);
  const terminalInstanceRef = useRef(null);
  const wsRef = useRef(null);

  const openTerminal = useCallback((containerData) => {
    setContainer(containerData);
    setError("");
  }, []);

  const closeTerminal = useCallback(() => {
    if (wsRef.current) {
      wsRef.current.close();
      wsRef.current = null;
    }
    if (terminalInstanceRef.current) {
      terminalInstanceRef.current.dispose();
      terminalInstanceRef.current = null;
    }
    setContainer(null);
    setConnecting(false);
    setError("");
  }, []);

  useEffect(() => {
    if (!container || !terminalRef.current || terminalInstanceRef.current) return;

    setConnecting(true);
    setError("");

    const term = new Terminal({
      cursorBlink: true,
      fontSize: 14,
      fontFamily: '"JetBrains Mono", monospace',
      theme: {
        background: '#1e1e2e',
        foreground: '#cdd6f4',
        cursor: '#f5e0dc',
        cursorAccent: '#1e1e2e',
        selectionBackground: '#585b70',
        black: '#45475a',
        red: '#f38ba8',
        green: '#a6e3a1',
        yellow: '#f9e2af',
        blue: '#89b4fa',
        magenta: '#f5c2e7',
        cyan: '#94e2d5',
        white: '#bac2de',
        brightBlack: '#585b70',
        brightRed: '#f38ba8',
        brightGreen: '#a6e3a1',
        brightYellow: '#f9e2af',
        brightBlue: '#89b4fa',
        brightMagenta: '#f5c2e7',
        brightCyan: '#94e2d5',
        brightWhite: '#a6adc8',
      },
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.open(terminalRef.current);
    fitAddon.fit();
    terminalInstanceRef.current = term;

    term.writeln('Connecting to container...');

    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${wsProtocol}//${window.location.host}/compute/containers/${container.id}/terminal`;
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onopen = () => {
      setConnecting(false);
      term.clear();
    };

    ws.onmessage = (event) => {
      if (event.data instanceof Blob) {
        event.data.text().then((text) => term.write(text));
      } else {
        term.write(event.data);
      }
    };

    ws.onerror = () => {
      setError('Connection error');
      setConnecting(false);
    };

    ws.onclose = (event) => {
      if (event.code !== 1000) {
        term.writeln('\r\n\x1b[31mConnection closed\x1b[0m');
      }
      setConnecting(false);
    };

    term.onData((data) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(data);
      }
    });

    const handleResize = () => fitAddon.fit();
    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
    };
  }, [container]);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (wsRef.current) wsRef.current.close();
      if (terminalInstanceRef.current) terminalInstanceRef.current.dispose();
    };
  }, []);

  return {
    container,
    connecting,
    error,
    terminalRef,
    openTerminal,
    closeTerminal,
  };
}
