import { createContext, useContext, useEffect, useState } from "react";
import { buildAuthBase, getAuthToken, setAuthToken, clearAuthToken } from "@/lib/api";
import { clearAllCaches } from "@/lib/cache";
import type { JwtPayload } from "@/types";

interface AuthContextValue {
  user: string | null;
  userId: string | null;
  displayName: string | null;
  isAdmin: boolean;
  loading: boolean;
  login: (username: string, password: string) => Promise<boolean>;
  logout: () => Promise<void>;
  checkSession: () => Promise<void>;
}

interface LoginResponse {
  token?: string;
  username: string;
  user_id?: string;
  display_name?: string;
  is_admin?: boolean;
}

interface SessionResponse {
  username: string;
  user_id?: string;
  display_name?: string;
  is_admin?: boolean;
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [user, setUser] = useState<string | null>(null);
  const [userId, setUserId] = useState<string | null>(null);
  const [displayName, setDisplayName] = useState<string | null>(null);
  const [isAdmin, setIsAdmin] = useState<boolean>(false);
  const [loading, setLoading] = useState<boolean>(true);

  // Decode JWT payload without validation (validation happens server-side)
  const decodeToken = (token: string): JwtPayload | null => {
    try {
      const parts = token.split(".");
      if (parts.length !== 3) return null;
      const payload: JwtPayload = JSON.parse(atob(parts[1]));
      // Check if token is expired
      if (payload.exp && payload.exp * 1000 < Date.now()) {
        return null;
      }
      return payload;
    } catch {
      return null;
    }
  };

  const checkSession = async (): Promise<void> => {
    const token = getAuthToken();
    if (!token) {
      setUser(null);
      setUserId(null);
      setDisplayName(null);
      setIsAdmin(false);
      setLoading(false);
      return;
    }

    // First, try to decode the token locally for immediate state
    const decoded = decodeToken(token);
    if (!decoded) {
      // Token is invalid or expired
      clearAuthToken();
      setUser(null);
      setUserId(null);
      setDisplayName(null);
      setIsAdmin(false);
      setLoading(false);
      return;
    }

    // Set state from decoded token immediately (optimistic)
    setUser(decoded.username);
    setUserId(decoded.user_id || null);
    setDisplayName(decoded.display_name || decoded.username);
    setLoading(false);

    // Then verify with server in background (for is_admin and fresh data)
    try {
      const response = await fetch(`${buildAuthBase()}/api/session`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (response.status === 401) {
        // Token rejected by server - clear it
        clearAuthToken();
        setUser(null);
        setUserId(null);
        setDisplayName(null);
        setIsAdmin(false);
        return;
      }
      if (response.ok) {
        const payload: SessionResponse = await response.json();
        setUser(payload.username);
        setUserId(payload.user_id || null);
        setDisplayName(payload.display_name || payload.username);
        setIsAdmin(payload.is_admin || false);
      }
      // On other errors (network, 500, etc), keep the decoded token state
    } catch (err) {
      // Network error - don't clear token, keep decoded state
      console.warn("Session check failed:", (err as Error).message);
    }
  };

  useEffect(() => {
    checkSession();
  }, []);

  const login = async (username: string, password: string): Promise<boolean> => {
    const response = await fetch(`${buildAuthBase()}/api/login`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username, password }),
    });
    if (!response.ok) {
      const error = await response.text();
      throw new Error(error || "Login failed");
    }
    const data: LoginResponse = await response.json();
    if (data.token) {
      setAuthToken(data.token);
    }
    setUser(data.username);
    setUserId(data.user_id || null);
    setDisplayName(data.display_name || data.username);
    setIsAdmin(data.is_admin || false);
    return true;
  };

  const logout = async (): Promise<void> => {
    const token = getAuthToken();
    try {
      await fetch(`${buildAuthBase()}/api/logout`, {
        method: "POST",
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
    } catch (err) {
      console.warn("Logout error:", err);
    }
    clearAuthToken();
    setUser(null);
    setUserId(null);
    setDisplayName(null);
    setIsAdmin(false);
    clearAllCaches();
  };

  const value: AuthContextValue = {
    user,
    userId,
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

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (context === undefined) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}
