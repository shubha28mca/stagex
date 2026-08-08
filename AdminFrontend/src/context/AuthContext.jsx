import { createContext, useContext, useEffect, useMemo, useState } from "react";
import { TOKEN_STORAGE_KEY, ADMIN_STORAGE_KEY } from "../config";

// AuthContext holds the logged-in admin (with role) and the JWT. The role drives
// which route tree the user may enter — Ops and Event Admin never overlap.
const AuthContext = createContext(null);

export function AuthProvider({ children }) {
  const [admin, setAdmin] = useState(() => {
    const raw = localStorage.getItem(ADMIN_STORAGE_KEY);
    return raw ? JSON.parse(raw) : null;
  });

  useEffect(() => {
    if (admin) localStorage.setItem(ADMIN_STORAGE_KEY, JSON.stringify(admin));
    else localStorage.removeItem(ADMIN_STORAGE_KEY);
  }, [admin]);

  const login = (auth) => {
    localStorage.setItem(TOKEN_STORAGE_KEY, auth.token);
    setAdmin(auth.admin);
  };
  const logout = () => {
    localStorage.removeItem(TOKEN_STORAGE_KEY);
    setAdmin(null);
  };

  const value = useMemo(
    () => ({ admin, role: admin?.role, isAuthenticated: !!admin, login, logout }),
    [admin]
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
