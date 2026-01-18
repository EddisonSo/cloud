import { useState, useRef, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { resolveApiHost } from "@/lib/api";
import { ArrowLeft, Server } from "lucide-react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";

export function NodeTerminal({ node, privilegedToken, onBack }) {
  const [connecting, setConnecting] = useState(true);
  const [error, setError] = useState("");
  const terminalRef = useRef(null);
  const terminalInstanceRef = useRef(null);
  const wsRef = useRef(null);

  useEffect(() => {
    if (!node || !terminalRef.current || terminalInstanceRef.current) return;

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

    term.writeln(`Connecting to node ${node.name}...`);

    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${wsProtocol}//${resolveApiHost()}/cluster-manager/nodes/${encodeURIComponent(node.name)}/terminal`;

    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    // Send privileged token as first message after connection
    ws.onopen = () => {
      // Send headers via query params or protocol isn't supported for WebSocket
      // We'll handle auth differently - the backend validates the session cookie
      setConnecting(false);
      term.clear();

      // Send initial resize
      const { cols, rows } = term;
      ws.send(JSON.stringify({ cols, rows }));
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

    term.onResize(({ cols, rows }) => {
      if (ws.readyState === WebSocket.OPEN) {
        ws.send(JSON.stringify({ cols, rows }));
      }
    });

    const handleResize = () => fitAddon.fit();
    window.addEventListener('resize', handleResize);

    return () => {
      window.removeEventListener('resize', handleResize);
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
      if (terminalInstanceRef.current) {
        terminalInstanceRef.current.dispose();
        terminalInstanceRef.current = null;
      }
    };
  }, [node, privilegedToken]);

  return (
    <div>
      <div className="flex items-center gap-4 mb-4">
        <Button variant="outline" size="sm" onClick={onBack}>
          <ArrowLeft className="w-4 h-4 mr-2" />
          Back
        </Button>
        <h2 className="text-xl font-semibold flex items-center gap-2">
          <Server className="w-5 h-5" />
          Terminal: {node?.name}
        </h2>
        {connecting && (
          <span className="text-sm text-muted-foreground">Connecting...</span>
        )}
      </div>

      <Card>
        <CardContent className="p-0">
          {error ? (
            <div className="p-6 text-center">
              <p className="text-destructive">{error}</p>
              <Button variant="outline" className="mt-4" onClick={onBack}>
                Go Back
              </Button>
            </div>
          ) : (
            <div
              ref={terminalRef}
              className="terminal-container min-h-[500px]"
            />
          )}
        </CardContent>
      </Card>
    </div>
  );
}
