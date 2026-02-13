import { createContext, useContext, useEffect, useState } from "react";
import { buildAuthBase, getAuthToken, setAuthToken, clearAuthToken } from "@/lib/api";
import { clearAllCaches } from "@/lib/cache";
import {
  parseRequestOptions,
  serializeAssertionResponse,
  getCredential,
} from "@/lib/webauthn";
import type { JwtPayload } from "@/types";

interface AuthContextValue {
  user: string | null;
  userId: string | null;
  displayName: string | null;
  isAdmin: boolean;
  loading: boolean;
  login: (username: string, password: string) => Promise<boolean | "2fa">;
  logout: () => Promise<void>;
  checkSession: () => Promise<void>;
  challengeToken: string | null;
  complete2FA: () => Promise<boolean>;
  cancel2FA: () => void;
}

interface LoginResponse {
  token?: string;
  username?: string;
  user_id?: string;
  display_name?: string;
  is_admin?: boolean;
  requires_2fa?: boolean;
  challenge_token?: string;
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
  const [challengeToken, setChallengeToken] = useState<string | null>(null);

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

  const login = async (username: string, password: string): Promise<boolean | "2fa"> => {
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

    if (data.requires_2fa && data.challenge_token) {
      setChallengeToken(data.challenge_token);
      return "2fa";
    }

    if (data.token) {
      setAuthToken(data.token);
    }
    setUser(data.username || null);
    setUserId(data.user_id || null);
    setDisplayName(data.display_name || data.username || null);
    setIsAdmin(data.is_admin || false);
    return true;
  };

  const complete2FA = async (): Promise<boolean> => {
    if (!challengeToken) throw new Error("No 2FA challenge in progress");

    // Step 1: Begin WebAuthn login
    const beginRes = await fetch(`${buildAuthBase()}/api/webauthn/login/begin`, {
      method: "POST",
      headers: { Authorization: `Bearer ${challengeToken}` },
    });
    if (!beginRes.ok) {
      throw new Error("Failed to start 2FA challenge");
    }
    const { options, state } = await beginRes.json();

    // Step 2: Get credential from browser
    const parsed = parseRequestOptions(options);
    const credential = await getCredential(parsed);
    const serialized = serializeAssertionResponse(credential);

    // Step 3: Finish WebAuthn login
    const finishRes = await fetch(`${buildAuthBase()}/api/webauthn/login/finish`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ state, credential: serialized }),
    });
    if (!finishRes.ok) {
      throw new Error("Security key verification failed");
    }
    const data: LoginResponse = await finishRes.json();

    if (data.token) {
      setAuthToken(data.token);
    }
    setUser(data.username || null);
    setUserId(data.user_id || null);
    setDisplayName(data.display_name || data.username || null);
    setIsAdmin(data.is_admin || false);
    setChallengeToken(null);
    return true;
  };

  const cancel2FA = () => {
    setChallengeToken(null);
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
    setChallengeToken(null);
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
    challengeToken,
    complete2FA,
    cancel2FA,
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
