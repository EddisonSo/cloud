import { useState, useEffect } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Monitor, RefreshCw } from "lucide-react";
import { fetchSessions } from "@/lib/settings-api";
import { formatTimestamp } from "@/lib/formatters";
import type { UserSession } from "@/types";

export function ActiveSessions() {
  const [sessions, setSessions] = useState<UserSession[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const load = async () => {
    try {
      setSessions(await fetchSessions());
    } catch {
      setError("Failed to load sessions");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { load(); }, []);

  if (loading) {
    return (
      <Card>
        <CardHeader>
          <Skeleton className="h-5 w-32" />
        </CardHeader>
        <CardContent className="space-y-2">
          {[...Array(2)].map((_, i) => (
            <div key={i} className="flex items-center gap-4 px-4 py-3 bg-secondary">
              <Skeleton className="h-5 w-32" />
              <Skeleton className="h-4 w-24 ml-auto" />
            </div>
          ))}
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader className="flex flex-row items-center justify-between">
        <CardTitle>Active Sessions</CardTitle>
        <Button variant="ghost" size="icon" className="h-8 w-8" onClick={load}>
          <RefreshCw className="w-4 h-4" />
        </Button>
      </CardHeader>
      <CardContent>
        {error && <p className="text-sm text-destructive mb-4">{error}</p>}

        {sessions.length === 0 ? (
          <div className="text-center py-8 text-muted-foreground">
            <Monitor className="w-8 h-8 mx-auto mb-2 opacity-40" />
            <p className="text-sm">No active sessions</p>
          </div>
        ) : (
          <div>
            {/* Header */}
            <div className="grid grid-cols-[1fr_140px] gap-4 px-4 py-3 border-b border-border">
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint">IP Address</div>
              <div className="font-mono text-[10px] uppercase tracking-[0.2em] text-faint text-center">Created</div>
            </div>
            {/* Rows */}
            {sessions.map((session) => (
              <div
                key={session.id}
                className="grid grid-cols-[1fr_140px] gap-4 px-4 py-3 border-b border-border last:border-0 hover:bg-popover transition-colors items-center"
              >
                <div className="flex items-center gap-2 min-w-0">
                  <Monitor className="w-4 h-4 text-faint shrink-0" />
                  <span className="font-mono text-[12.5px] text-muted-foreground truncate">{session.ip_address || "Unknown"}</span>
                  {session.is_current && (
                    <Badge variant="success" className="ml-1 text-[10px] px-1.5 py-0">
                      Current
                    </Badge>
                  )}
                </div>
                <div className="text-center font-mono text-[12.5px] text-muted-foreground">
                  {formatTimestamp(session.created_at)}
                </div>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
