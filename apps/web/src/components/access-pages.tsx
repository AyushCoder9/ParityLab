"use client";

import Link from "next/link";
import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { BrandMark, Icon, StatusPill } from "@paritylab/ui";
import { APIRequestError, getSession, login, register } from "@/lib/api";
import { useAuth } from "./auth-context";

export function LoginPage() {
  const router = useRouter();
  const [mode, setMode] = useState<"login" | "register">("login");
  const [email, setEmail] = useState(""); const [password, setPassword] = useState("");
  const [workspaceName, setWorkspaceName] = useState(""); const [projectName, setProjectName] = useState("");
  const [pending, setPending] = useState(false); const [error, setError] = useState(""); const [checking, setChecking] = useState(true);

  useEffect(() => {
    getSession()
      .then(() => router.replace("/dashboard"))
      .catch((value) => {
        if (!(value instanceof APIRequestError) || value.status !== 401) {
          setError("The authentication service is unavailable. You can retry when the API is online.");
        }
        setChecking(false);
      });
  }, [router]);

  const submit = async (event: React.FormEvent) => {
    event.preventDefault(); setError("");
    if (password.length < 12) { setError("Password must contain at least 12 characters."); return; }
    setPending(true);
    try {
      if (mode === "login") await login({ email, password });
      else await register({ email, password, workspaceName, projectName });
      const requested = new URLSearchParams(window.location.search).get("next");
      router.replace(requested?.startsWith("/") && !requested.startsWith("//") ? requested : "/dashboard");
      router.refresh();
    } catch (value) {
      setError(value instanceof APIRequestError ? value.message : "The authentication service is unavailable. Please try again.");
    } finally { setPassword(""); setPending(false); }
  };

  return <main className="access-page"><header><Link href="/"><BrandMark /></Link><span>Secure, sandbox-only workspace</span></header><section className="access-card auth-card"><div className="auth-mode" role="tablist" aria-label="Authentication mode"><button type="button" role="tab" aria-selected={mode === "login"} onClick={() => { setMode("login"); setError(""); }}>Sign in</button><button type="button" role="tab" aria-selected={mode === "register"} onClick={() => { setMode("register"); setError(""); }}>Create workspace</button></div><span className="access-index mono">HTTPONLY SESSION</span><h1>{mode === "login" ? "Sign in to the control plane." : "Create your verification workspace."}</h1><p>{mode === "login" ? "Continue with the secure session stored by the ParityLab API." : "Create an owner account, organization, and first project in one transaction."}</p>{checking ? <div className="auth-check" role="status">Checking existing session…</div> : <form className="auth-form" onSubmit={submit}>{error ? <div className="inline-error" role="alert">{error}</div> : null}{mode === "register" ? <div className="auth-field-row"><label>Workspace name<input required autoComplete="organization" value={workspaceName} onChange={(event) => setWorkspaceName(event.target.value)} /></label><label>Project name<input required autoComplete="off" value={projectName} onChange={(event) => setProjectName(event.target.value)} /></label></div> : null}<label>Email address<input required type="email" autoComplete="email" value={email} onChange={(event) => setEmail(event.target.value)} /></label><label>Password<input required minLength={12} maxLength={256} type="password" autoComplete={mode === "login" ? "current-password" : "new-password"} value={password} onChange={(event) => setPassword(event.target.value)} aria-describedby="password-requirement" /></label><small id="password-requirement">Use 12–256 characters.</small><button className="button button--primary" disabled={pending}>{pending ? "Securing session…" : mode === "login" ? "Sign in" : "Create workspace"}<Icon name="arrow"/></button></form>}<div className="access-safety"><Icon name="check"/><span><strong>No tokens in browser storage</strong><small>The API owns the HttpOnly session cookie. Live Stripe mode remains blocked.</small></span></div><Link className="text-link" href="/">Return to website</Link></section></main>;
}

export function OnboardingPage() {
  const { session } = useAuth();
  const [step, setStep] = useState(1);
  const [copied, setCopied] = useState(false);
  const copyTemplate = async () => { await navigator.clipboard.writeText("PARITYLAB_ENCRYPTION_KEY=generate-32-byte-key\nNEXT_PUBLIC_PARITYLAB_API_URL=http://127.0.0.1:8080"); setCopied(true); };
  return <main className="access-page onboarding-page"><header><Link href="/"><BrandMark /></Link><Link href="/dashboard">Exit setup</Link></header><section className="onboarding-shell"><aside><span className="mono">SETUP / 03</span><h1>Prepare a safe verification workspace.</h1><ol>{["Workspace", "API safety", "Stripe Sandbox"].map((label, index) => <li key={label} className={step === index + 1 ? "is-active" : step > index + 1 ? "is-complete" : ""}><span>{step > index + 1 ? "✓" : index + 1}</span>{label}</li>)}</ol></aside><div className="onboarding-content">{step === 1 ? <><StatusPill tone="verified">Authenticated workspace</StatusPill><h2>{session?.organization.name}</h2><p>Your account, organization, and project are persisted by the API. This setup configures <strong>{session?.project.name}</strong>.</p><div className="onboarding-record"><span>Signed in as</span><strong>{session?.user.email}</strong><span>Organization role</span><strong>{session?.organization.role}</strong></div><button className="button button--primary" onClick={() => setStep(2)}>Continue <Icon name="arrow"/></button></> : step === 2 ? <><StatusPill tone="warning">Server prerequisite</StatusPill><h2>Enable encrypted connection storage</h2><p>The API needs its encryption key before it can accept a restricted Stripe Sandbox key. Put this non-Stripe configuration in an ignored <code>.env.local</code> file and restart the engine.</p><pre>PARITYLAB_ENCRYPTION_KEY=generate-32-byte-key{"\n"}NEXT_PUBLIC_PARITYLAB_API_URL=http://127.0.0.1:8080</pre><button className="button button--secondary" onClick={copyTemplate}>{copied ? "Template copied" : "Copy environment template"}</button><button className="button button--primary" onClick={() => setStep(3)}>Continue to secure connection <Icon name="arrow"/></button></> : <><StatusPill tone="neutral">Secure API handoff</StatusPill><h2>Validate a restricted Sandbox key</h2><p>The Connections screen sends the key directly to the API, clears the input immediately, and renders only the sanitized connection record returned by the backend.</p><Link className="button button--primary" href="/connections">Open secure Stripe connection <Icon name="arrow"/></Link><button className="button button--quiet" onClick={() => setStep(2)}>Back to configuration</button></>}</div></section></main>;
}
