import { createContext, useContext, useEffect, useMemo, useState } from "react";
import { TOKEN_STORAGE_KEY, FAMILY_STORAGE_KEY } from "../config";

// AuthContext holds the logged-in family and the JWT. The token is persisted to
// localStorage; api/client.js reads it from there for the Authorization header,
// so components never pass tokens around manually.
const AuthContext = createContext(null);

export function AuthProvider({ children }) {
  const [family, setFamily] = useState(() => {
    const raw = localStorage.getItem(FAMILY_STORAGE_KEY);
    return raw ? JSON.parse(raw) : null;
  });

  // Keep localStorage in sync so a page refresh preserves the session.
  useEffect(() => {
    if (family) localStorage.setItem(FAMILY_STORAGE_KEY, JSON.stringify(family));
    else localStorage.removeItem(FAMILY_STORAGE_KEY);
  }, [family]);

  const login = (auth) => {
    localStorage.setItem(TOKEN_STORAGE_KEY, auth.token);
    setFamily(auth.family);
  };

  const logout = () => {
    localStorage.removeItem(TOKEN_STORAGE_KEY);
    setFamily(null);
  };

  const value = useMemo(
    () => ({ family, isAuthenticated: !!family, login, logout }),
    [family]
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

// useAuth is the hook every component uses to read/change auth state.
export function useAuth() {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
