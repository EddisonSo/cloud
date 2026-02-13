import React from "react";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { ArrowLeft } from "lucide-react";
import "@xterm/xterm/css/xterm.css";
import type { Container } from "@/types";

interface TerminalViewProps {
  container: Container;
  terminalRef: React.RefObject<HTMLDivElement | null>;
  connecting: boolean;
  error: string;
  onClose: () => void;
}

export function TerminalView({
  container,
  terminalRef,
  connecting,
  error,
  onClose,
}: TerminalViewProps) {
  return (
    <div>
      <div className="flex items-center gap-4 mb-4">
        <Button variant="outline" size="sm" onClick={onClose}>
          <ArrowLeft className="w-4 h-4 mr-2" />
          Back
        </Button>
        <h2 className="text-xl font-semibold">Terminal: {container?.name}</h2>
        {connecting && (
          <span className="text-sm text-muted-foreground">Connecting...</span>
        )}
      </div>

      <Card>
        <CardContent className="p-0">
          {error ? (
            <div className="p-6 text-center">
              <p className="text-destructive">{error}</p>
              <Button variant="outline" className="mt-4" onClick={onClose}>
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
