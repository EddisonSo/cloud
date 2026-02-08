import { createContext, useContext, useEffect, useState, useCallback, useRef } from "react";
import { buildNotificationsBase, buildNotificationsWsBase, getAuthToken } from "@/lib/api";
import { useAuth } from "@/contexts/AuthContext";

const NotificationContext = createContext();

export function NotificationProvider({ children }) {
  const { user } = useAuth();
  const [notifications, setNotifications] = useState([]);
  const [unreadCount, setUnreadCount] = useState(0);
  const wsRef = useRef(null);
  const reconnectTimer = useRef(null);

  const fetchNotifications = useCallback(async () => {
    const token = getAuthToken();
    if (!token) return;
    try {
      const res = await fetch(`${buildNotificationsBase()}/api/notifications?limit=20`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setNotifications(data);
      }
    } catch (err) {
      console.warn("Failed to fetch notifications:", err.message);
    }
  }, []);

  const fetchUnreadCount = useCallback(async () => {
    const token = getAuthToken();
    if (!token) return;
    try {
      const res = await fetch(`${buildNotificationsBase()}/api/notifications/unread-count`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setUnreadCount(data.count);
      }
    } catch (err) {
      console.warn("Failed to fetch unread count:", err.message);
    }
  }, []);

  const markAsRead = useCallback(async (id) => {
    const token = getAuthToken();
    if (!token) return;
    try {
      await fetch(`${buildNotificationsBase()}/api/notifications/${id}/read`, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}` },
      });
      setNotifications((prev) =>
        prev.map((n) => (n.id === id ? { ...n, read: true } : n))
      );
      setUnreadCount((prev) => Math.max(0, prev - 1));
    } catch (err) {
      console.warn("Failed to mark notification as read:", err.message);
    }
  }, []);

  const markAllRead = useCallback(async () => {
    const token = getAuthToken();
    if (!token) return;
    try {
      await fetch(`${buildNotificationsBase()}/api/notifications/read-all`, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}` },
      });
      setNotifications((prev) => prev.map((n) => ({ ...n, read: true })));
      setUnreadCount(0);
    } catch (err) {
      console.warn("Failed to mark all as read:", err.message);
    }
  }, []);

  const connectWebSocket = useCallback(() => {
    const token = getAuthToken();
    if (!token) return;

    const wsUrl = `${buildNotificationsWsBase()}/ws/notifications?token=${encodeURIComponent(token)}`;
    const ws = new WebSocket(wsUrl);
    wsRef.current = ws;

    ws.onmessage = (event) => {
      try {
        const notification = JSON.parse(event.data);
        setNotifications((prev) => [notification, ...prev].slice(0, 50));
        setUnreadCount((prev) => prev + 1);
      } catch (err) {
        console.warn("Failed to parse notification:", err.message);
      }
    };

    ws.onclose = () => {
      wsRef.current = null;
      // Reconnect with exponential backoff
      reconnectTimer.current = setTimeout(connectWebSocket, 5000);
    };

    ws.onerror = () => {
      ws.close();
    };
  }, []);

  useEffect(() => {
    if (!user) {
      setNotifications([]);
      setUnreadCount(0);
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
      if (reconnectTimer.current) {
        clearTimeout(reconnectTimer.current);
      }
      return;
    }

    fetchNotifications();
    fetchUnreadCount();
    connectWebSocket();

    return () => {
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
      if (reconnectTimer.current) {
        clearTimeout(reconnectTimer.current);
      }
    };
  }, [user, fetchNotifications, fetchUnreadCount, connectWebSocket]);

  const value = {
    notifications,
    unreadCount,
    markAsRead,
    markAllRead,
    fetchNotifications,
  };

  return (
    <NotificationContext.Provider value={value}>
      {children}
    </NotificationContext.Provider>
  );
}

export function useNotifications() {
  const context = useContext(NotificationContext);
  if (context === undefined) {
    throw new Error("useNotifications must be used within a NotificationProvider");
  }
  return context;
}
