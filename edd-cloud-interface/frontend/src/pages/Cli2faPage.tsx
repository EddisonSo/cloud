import { useEffect, useState } from "react";
import { useSearchParams } from "react-router-dom";
import { buildAuthBase, copyToClipboard } from "@/lib/api";
import { parseRequestOptions, getCredential, serializeAssertionResponse } from "@/lib/webauthn";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

type Phase = "idle" | "verifying" | "done" | "error";

// Only post the captured token to a loopback callback (the local ec listener).
function isLoopbackCallback(cb: string): boolean {
  try {
    const u = new URL(cb);
    return u.protocol === "http:" && (u.hostname === "127.0.0.1" || u.hostname === "localhost");
  } catch {
    return false;
  }
}

export function Cli2faPage() {
  const [params] = useSearchParams();
  const challenge = params.get("challenge") || "";
  const cb = params.get("cb") || "";
  const [phase, setPhase] = useState<Phase>("idle");
  const [token, setToken] = useState("");
  const [error, setError] = useState("");

  // Defense-in-depth: prevent the challenge/nonce in this page's URL from
  // leaking via the Referer header to any subresource. Scoped to this page.
  useEffect(() => {
    const meta = document.createElement("meta");
    meta.name = "referrer";
    meta.content = "no-referrer";
    document.head.appendChild(meta);
    return () => {
      document.head.removeChild(meta);
    };
  }, []);

  async function verify() {
    if (!challenge) {
      setPhase("error");
      setError("Missing challenge. Re-run `ec auth login`.");
      return;
    }
    setPhase("verifying");
    try {
      const beginRes = await fetch(`${buildAuthBase()}/api/webauthn/login/begin`, {
        method: "POST",
        headers: { Authorization: `Bearer ${challenge}` },
      });
      if (!beginRes.ok) throw new Error("Challenge expired or invalid. Re-run `ec auth login`.");
      const { options, state } = await beginRes.json();

      const parsed = parseRequestOptions(options);
      const credential = await getCredential(parsed);
      const serialized = serializeAssertionResponse(credential);

      const finishRes = await fetch(`${buildAuthBase()}/api/webauthn/login/finish`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ state, credential: serialized }),
      });
      if (!finishRes.ok) throw new Error("Security key verification failed.");
      const data = await finishRes.json();
      const sessionToken: string = data.token;
      if (!sessionToken) throw new Error("Server error: no session token returned.");

      setToken(sessionToken);
      setPhase("done");

      if (cb && isLoopbackCallback(cb)) {
        // Best-effort handoff to the local ec listener; ignore failures (e.g. over SSH).
        fetch(cb, { method: "POST", body: sessionToken }).catch(() => {});
      }
    } catch (e) {
      setPhase("error");
      setError((e as Error).message);
    }
  }

  return (
    <div className="min-h-screen flex items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>CLI Security Key Verification</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          {phase === "idle" && (
            <>
              <p className="text-sm text-muted-foreground">
                Verify your security key to finish logging in to the <code>ec</code> CLI.
              </p>
              <Button onClick={verify} disabled={!challenge}>Verify security key</Button>
              {!challenge && (
                <p className="text-sm text-destructive">
                  Missing challenge. Re-run <code>ec auth login</code>.
                </p>
              )}
            </>
          )}
          {phase === "verifying" && (
            <p className="text-sm text-muted-foreground">Waiting for your security key…</p>
          )}
          {phase === "done" && (
            <>
              <p className="text-sm text-muted-foreground">
                Verified. Paste this token into your terminal:
              </p>
              <div className="flex gap-2">
                <Input readOnly value={token} className="font-mono text-xs" />
                <Button onClick={() => copyToClipboard(token)}>Copy</Button>
              </div>
              <p className="text-xs text-muted-foreground">
                If <code>ec</code> logged you in automatically, you can close this tab.
              </p>
            </>
          )}
          {phase === "error" && (
            <>
              <p className="text-sm text-destructive">{error}</p>
              <Button variant="outline" onClick={() => { setPhase("idle"); setError(""); }}>
                Try again
              </Button>
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
