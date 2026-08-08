import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { authApi } from "../api/endpoints";
import { useAuth } from "../context/AuthContext";
import { Button, Field, Alert } from "../components/ui";

// LoginPage authenticates an admin and routes them to their role's area.
// The seeded mock credentials are shown so the console is testable immediately.
const MOCKS = {
  ops: { email: "ops@stagex.test", password: "Ops@12345", label: "Operational Admin" },
  event: { email: "event@stagex.test", password: "Event@12345", label: "Event Admin" },
};

export default function LoginPage() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const submit = async () => {
    setError("");
    setBusy(true);
    try {
      const auth = await authApi.login(email, password);
      login(auth);
      navigate(auth.admin.role === "ops" ? "/ops" : "/event");
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  const fill = (which) => {
    setEmail(MOCKS[which].email);
    setPassword(MOCKS[which].password);
  };

  return (
    <div className="loginwrap">
      <div className="logincard">
        <div className="logo" style={{ marginBottom: 6 }}>
          <span className="mark">✦</span>
          <span>StageX<small>Admin Console</small></span>
        </div>
        <p className="muted mb" style={{ fontSize: ".85rem" }}>Sign in to your admin account.</p>

        <Alert>{error}</Alert>

        <Field label="Email" value={email} onChange={setEmail} type="email" />
        <Field label="Password" value={password} onChange={setPassword} type="password" />
        <Button block onClick={submit} disabled={busy || !email || !password}>Log in</Button>

        <div className="mt" style={{ borderTop: "1px solid var(--line)", paddingTop: 12 }}>
          <p className="muted" style={{ fontSize: ".72rem", marginBottom: 8 }}>Demo accounts — click to fill:</p>
          <div className="role-tabs">
            <button className="chip" onClick={() => fill("ops")}>Operational Admin</button>
            <button className="chip" onClick={() => fill("event")}>Event Admin</button>
          </div>
        </div>
      </div>
    </div>
  );
}
