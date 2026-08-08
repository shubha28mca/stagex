import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { authApi } from "../api/endpoints";
import { useAuth } from "../context/AuthContext";
import { Button, Field, Panel, Alert } from "../components";

// AuthPage — phone + OTP registration and dual (password / OTP) login, matching
// ClientDesignWeb §3. In development the backend returns the OTP, which we show
// as a hint so the flow is testable without an SMS gateway.
export default function AuthPage() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const [mode, setMode] = useState("login"); // login | register
  const [loginMethod, setLoginMethod] = useState("password"); // password | otp

  const [phone, setPhone] = useState("");
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [otp, setOtp] = useState("");
  const [otpSent, setOtpSent] = useState(false);
  const [devOtp, setDevOtp] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  const purpose = mode === "register" ? "register" : "login";

  const sendOtp = async () => {
    setError("");
    setBusy(true);
    try {
      const res = await authApi.sendOtp(phone, purpose);
      setOtpSent(true);
      if (res.devOtp) {
        setDevOtp(res.devOtp);
        setOtp(res.devOtp); // pre-fill in dev for convenience
      }
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  const submit = async () => {
    setError("");
    setBusy(true);
    try {
      let auth;
      if (mode === "register") {
        auth = await authApi.register(phone, name, password, otp);
      } else if (loginMethod === "password") {
        auth = await authApi.login(phone, { password });
      } else {
        auth = await authApi.login(phone, { otp });
      }
      login(auth);
      navigate("/discover");
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  const needsOtp = mode === "register" || (mode === "login" && loginMethod === "otp");

  return (
    <div style={{ maxWidth: 440, margin: "24px auto" }}>
      <Panel title={mode === "register" ? "Create your family account" : "Welcome back"}>
        <div className="row" style={{ marginBottom: 14, background: "var(--cloud)", borderRadius: 12, padding: 4 }}>
          <button className={`tab ${mode === "login" ? "on" : ""}`} style={{ flex: 1 }} onClick={() => setMode("login")}>
            Login
          </button>
          <button className={`tab ${mode === "register" ? "on" : ""}`} style={{ flex: 1 }} onClick={() => setMode("register")}>
            Register
          </button>
        </div>

        <Alert>{error}</Alert>

        <Field
          label="Mobile number"
          value={phone}
          onChange={setPhone}
          inputMode="numeric"
          maxLength={10}
          hint="10-digit Indian mobile"
        />

        {mode === "register" && (
          <Field label="Your name" value={name} onChange={setName} />
        )}

        {mode === "login" && (
          <div className="row" style={{ marginBottom: 12 }}>
            <button className={`chip ${loginMethod === "password" ? "on" : ""}`} onClick={() => setLoginMethod("password")}>
              Use password
            </button>
            <button className={`chip ${loginMethod === "otp" ? "on" : ""}`} onClick={() => setLoginMethod("otp")}>
              Use OTP
            </button>
          </div>
        )}

        {(mode === "register" || loginMethod === "password") && (
          <Field
            label="Password"
            type="password"
            value={password}
            onChange={setPassword}
            hint={mode === "register" ? "8+ characters" : undefined}
          />
        )}

        {needsOtp && (
          <>
            {!otpSent ? (
              <Button variant="o" block onClick={sendOtp} disabled={busy || phone.length !== 10}>
                Send OTP
              </Button>
            ) : (
              <>
                <Field label="Enter OTP" value={otp} onChange={setOtp} inputMode="numeric" maxLength={6} />
                {devOtp && <div className="fmsg ok">Dev OTP: {devOtp}</div>}
              </>
            )}
          </>
        )}

        <div className="mt">
          <Button
            variant="g"
            block
            onClick={submit}
            disabled={busy || (needsOtp && !otpSent)}
          >
            {mode === "register" ? "Create account" : "Log in"}
          </Button>
        </div>
      </Panel>
    </div>
  );
}
