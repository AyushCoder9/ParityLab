"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { usePathname, useRouter } from "next/navigation";
import { Icon } from "@paritylab/ui";
import { APIRequestError, getSession, type SessionView } from "@/lib/api";

type AuthState = {
  status: "loading" | "authenticated" | "unauthenticated" | "unavailable";
  session: SessionView | null;
  refresh: () => Promise<void>;
};

const AuthContext = createContext<AuthState | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [status, setStatus] = useState<AuthState["status"]>("loading");
  const [session, setSession] = useState<SessionView | null>(null);

  const refresh = useCallback(async () => {
    setStatus((current) => current === "authenticated" ? current : "loading");
    try {
      const value = await getSession();
      setSession(value);
      setStatus("authenticated");
    } catch (error) {
      setSession(null);
      setStatus(error instanceof APIRequestError && error.status === 401 ? "unauthenticated" : "unavailable");
    }
  }, []);

  useEffect(() => { void refresh(); }, [refresh]);
  const value = useMemo(() => ({ status, session, refresh }), [refresh, session, status]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used inside AuthProvider");
  return value;
}

export function ProtectedProduct({ children }: { children: React.ReactNode }) {
  const { status, refresh } = useAuth();
  const pathname = usePathname();
  const router = useRouter();

  useEffect(() => {
    if (status === "unauthenticated") router.replace(`/login?next=${encodeURIComponent(pathname)}`);
  }, [pathname, router, status]);

  if (status === "authenticated") return children;
  if (status === "unavailable") return <main className="auth-gate"><div><Icon name="finding"/><span className="mono">SESSION UNAVAILABLE</span><h1>We could not verify your session.</h1><p>The API may be offline. No local identity or cached token was substituted.</p><button className="button button--primary" onClick={() => void refresh()}>Retry session check</button><a className="button button--secondary" href="/">Return to website</a></div></main>;
  return <main className="auth-gate" role="status"><div className="auth-gate__loading"><span/><span/><span/><p>Verifying secure session…</p></div></main>;
}
