import { createContext, useContext, useEffect, useState } from "react";
import { buildApiBase } from "@/lib/api";

const AuthContext = createContext();

export function AuthProvider({ children }) {
  const [user, setUser] = useState(null);
  const [isAdmin, setIsAdmin] = useState(false);
  const [loading, setLoading] = useState(true);

  const checkSession = async () => {
    try {
      const response = await fetch(`${buildApiBase()}/api/session`, {
        credentials: "include",
      });
      if (!response.ok) {
        setUser(null);
        setIsAdmin(false);
        return;
      }
      const payload = await response.json();
      setUser(payload.username);
      setIsAdmin(payload.is_admin || false);
    } catch (err) {
      setUser(null);
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
      credentials: "include",
      body: JSON.stringify({ username, password }),
    });
    if (!response.ok) {
      const error = await response.text();
      throw new Error(error || "Login failed");
    }
    await checkSession();
    return true;
  };

  const logout = async () => {
    try {
      await fetch(`${buildApiBase()}/api/logout`, {
        method: "POST",
        credentials: "include",
      });
    } catch (err) {
      console.warn("Logout error:", err);
    }
    setUser(null);
    setIsAdmin(false);
  };

  const value = {
    user,
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
