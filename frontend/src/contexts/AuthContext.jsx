import { createContext, useContext, useEffect, useState } from "react";
import { buildApiBase, getAuthToken, setAuthToken, clearAuthToken } from "@/lib/api";
import { clearAllCaches } from "@/lib/cache";

const AuthContext = createContext();

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null);
  const [displayName, setDisplayName] = useState(null);
  const [isAdmin, setIsAdmin] = useState(false);
  const [loading, setLoading] = useState(true);

  const checkSession = async () => {
    const token = getAuthToken();
    if (!token) {
      setUser(null);
      setDisplayName(null);
      setIsAdmin(false);
      setLoading(false);
      return;
    }

    try {
      const response = await fetch(`${buildApiBase()}/api/session`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (!response.ok) {
        clearAuthToken();
        setUser(null);
        setDisplayName(null);
        setIsAdmin(false);
        return;
      }
      const payload = await response.json();
      setUser(payload.username);
      setDisplayName(payload.display_name || payload.username);
      setIsAdmin(payload.is_admin || false);
    } catch (err) {
      clearAuthToken();
      setUser(null);
      setDisplayName(null);
      setIsAdmin(false);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    checkSession();
  }, []);

  const login = async (username, password) => {
    const response = await fetch(`${buildApiBase()}/api/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
    });
    if (!response.ok) {
      const error = await response.text();
      throw new Error(error || "Login failed");
    }
    const data = await response.json();
    if (data.token) {
      setAuthToken(data.token);
    }
    setUser(data.username);
    setDisplayName(data.display_name || data.username);
    setIsAdmin(data.is_admin || false);
    return true;
  };

  const logout = async () => {
    const token = getAuthToken();
    try {
      await fetch(`${buildApiBase()}/api/logout`, {
        method: "POST",
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
    } catch (err) {
      console.warn("Logout error:", err);
    }
    clearAuthToken();
    setUser(null);
    setDisplayName(null);
    setIsAdmin(false);
    clearAllCaches();
  };

  const value = {
    user,
    displayName,
    isAdmin,
    loading,
    login,
    logout,
    checkSession,
  };

  return (
    <AuthContext.Provider value={value}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}
