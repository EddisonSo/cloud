import { useState, useEffect, useCallback } from "react";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Switch } from "@/components/ui/switch";
import { Skeleton } from "@/components/ui/skeleton";
import { buildNotificationsBase, buildStorageBase, getAuthHeaders } from "@/lib/api";
import { BellOff, HardDrive } from "lucide-react";

export function NotificationMutes() {
  const [namespaces, setNamespaces] = useState([]);
  const [mutes, setMutes] = useState(new Set());
  const [loading, setLoading] = useState(true);
  const [toggling, setToggling] = useState(null);

  const loadData = useCallback(async () => {
    try {
      const [nsRes, mutesRes] = await Promise.all([
        fetch(`${buildStorageBase()}/storage/namespaces`, { headers: getAuthHeaders() }),
        fetch(`${buildNotificationsBase()}/api/notifications/mutes`, { headers: getAuthHeaders() }),
      ]);

      if (nsRes.ok) {
        const nsData = await nsRes.json();
        setNamespaces(nsData.sort((a, b) => a.name.localeCompare(b.name)));
      }

      if (mutesRes.ok) {
        const mutesData = await mutesRes.json();
        const mutedSet = new Set(
          mutesData
            .filter((m) => m.category === "storage")
            .map((m) => m.scope)
        );
        setMutes(mutedSet);
      }
    } catch (err) {
      console.error("Failed to load notification preferences", err);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    loadData();
  }, [loadData]);

  const toggleMute = async (namespace) => {
    const isMuted = mutes.has(namespace);
    setToggling(namespace);

    try {
      const res = await fetch(`${buildNotificationsBase()}/api/notifications/mutes`, {
        method: isMuted ? "DELETE" : "PUT",
        headers: { "Content-Type": "application/json", ...getAuthHeaders() },
        body: JSON.stringify({ category: "storage", scope: namespace }),
      });

      if (res.ok) {
        setMutes((prev) => {
          const next = new Set(prev);
          if (isMuted) {
            next.delete(namespace);
          } else {
            next.add(namespace);
          }
          return next;
        });
      }
    } catch (err) {
      console.error("Failed to update mute", err);
    } finally {
      setToggling(null);
    }
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <BellOff className="w-4 h-4" />
          Notifications
        </CardTitle>
        <CardDescription>
          Mute notifications from specific storage namespaces. Muted namespaces won't generate upload or delete notifications.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {loading ? (
          <div className="space-y-3">
            {[1, 2, 3].map((i) => (
              <Skeleton key={i} className="h-10 w-full" />
            ))}
          </div>
        ) : namespaces.length === 0 ? (
          <p className="text-sm text-muted-foreground">No storage namespaces found.</p>
        ) : (
          <div className="space-y-1">
            {namespaces.map((ns) => (
              <div
                key={ns.name}
                className="flex items-center justify-between py-2.5 px-3 rounded-md hover:bg-accent/50 transition-colors"
              >
                <div className="flex items-center gap-3">
                  <HardDrive className="w-4 h-4 text-muted-foreground" />
                  <span className="text-sm font-medium">{ns.name}</span>
                </div>
                <div className="flex items-center gap-3">
                  <span className="text-xs text-muted-foreground">
                    {mutes.has(ns.name) ? "Muted" : "Active"}
                  </span>
                  <Switch
                    checked={!mutes.has(ns.name)}
                    onCheckedChange={() => toggleMute(ns.name)}
                    disabled={toggling === ns.name}
                  />
                </div>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
