import { useEffect, useState, type FormEvent } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { ArrowRight, LoaderCircle, ShieldCheck } from "lucide-react";
import { CyberAgentClient } from "../api/client";
import { desktopBridgeAvailable, desktopErrorMessage, loadDesktopBootstrap } from "../lib/desktop-bridge";
import { useConnectionStore } from "../state/connection";

export function ConnectionGate() {
  const [token, setToken] = useState("");
  const [controlToken, setControlToken] = useState("");
  const [error, setError] = useState("");
  const [connecting, setConnecting] = useState(false);
  const connect = useConnectionStore((state) => state.connect);
  const queryClient = useQueryClient();

  useEffect(() => {
    if (!desktopBridgeAvailable()) {
      return;
    }
    let active = true;
    setConnecting(true);
    setError("");
    void loadDesktopBootstrap().then(async (bootstrap) => {
      if (!bootstrap || !active) {
        return;
      }
      const client = new CyberAgentClient(bootstrap.read_token, bootstrap.api_base_url,
        bootstrap.control_token);
      const health = await client.health();
      if (!active) {
        return;
      }
      queryClient.clear();
      connect(bootstrap.read_token, health, bootstrap.control_token, {
        runControlEnabled: bootstrap.control_enabled,
        runCreationEnabled: bootstrap.run_creation_enabled,
        sessionMessageEnabled: bootstrap.session_message_enabled,
        sessionSteeringControlEnabled: bootstrap.session_steering_control_enabled,
        runLifecycleEnabled: bootstrap.run_lifecycle_enabled,
        runExecutionEnabled: bootstrap.run_execution_enabled,
        planDeliveryControlEnabled: bootstrap.plan_delivery_control_enabled,
        approvalControlEnabled: bootstrap.approval_control_enabled,
		modelControlEnabled: bootstrap.model_control_enabled,
		providerCredentialEnabled: bootstrap.provider_credential_enabled,
		fileEditReviewEnabled: bootstrap.file_edit_review_enabled,
		fileEditProposalEnabled: bootstrap.file_edit_proposal_enabled,
		fileEditApplyEnabled: bootstrap.file_edit_apply_enabled,
		runWakeControlEnabled: bootstrap.run_wake_control_enabled,
		runWakeExecutionEnabled: bootstrap.run_wake_execution_enabled,
		runWakeWorkerEnabled: bootstrap.run_wake_worker_enabled,
		skillInstallationEnabled: bootstrap.skill_installation_enabled,
		evidenceAttachmentEnabled: bootstrap.evidence_attachment_enabled,
      });
    }).catch((caught: unknown) => {
      if (active) {
        setError(desktopErrorMessage(caught));
      }
    }).finally(() => {
      if (active) {
        setConnecting(false);
      }
    });
    return () => {
      active = false;
    };
  }, [connect, queryClient]);

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const candidate = token.trim();
    if (!candidate || connecting) {
      return;
    }
    setConnecting(true);
    setError("");
    try {
      const client = new CyberAgentClient(candidate);
      const health = await client.health();
      queryClient.clear();
      connect(candidate, health, controlToken.trim());
      setToken("");
      setControlToken("");
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "无法连接 Go 控制面");
    } finally {
      setConnecting(false);
    }
  };

  return (
    <main className="connection-page">
      <form className="connection-panel" onSubmit={submit}>
        <div className="brand-lockup">
          <span className="brand-mark"><ShieldCheck aria-hidden="true" size={24} /></span>
          <span>
            <strong>CyberAgent Workbench</strong>
            <small>Local control console</small>
          </span>
        </div>
        <div className="connection-heading">
          <h1>连接本地控制面</h1>
          <p>Go API / api.v1</p>
        </div>
        {connecting && desktopBridgeAvailable() &&
          <div className="desktop-connecting"><LoaderCircle aria-hidden="true" className="spin" size={16} />启动桌面工作台</div>}
        <label className="field-label" htmlFor="read-token">Read bearer token</label>
        <div className="token-row">
          <input
            autoCapitalize="none"
            autoComplete="off"
            autoCorrect="off"
            id="read-token"
            name="read-token"
            onChange={(event) => setToken(event.target.value)}
            placeholder="CYBERAGENT_API_TOKEN"
            spellCheck={false}
            type="password"
            value={token}
          />
          <button aria-label="连接" disabled={!token.trim() || connecting} title="连接" type="submit">
            {connecting ? <LoaderCircle aria-hidden="true" className="spin" size={18} /> : <ArrowRight aria-hidden="true" size={18} />}
          </button>
        </div>
        <label className="field-label optional-token-label" htmlFor="control-token">
          Control bearer token <span>optional</span>
        </label>
        <input
          autoCapitalize="none"
          autoComplete="off"
          autoCorrect="off"
          className="control-token-input"
          id="control-token"
          name="control-token"
          onChange={(event) => setControlToken(event.target.value)}
          placeholder="CYBERAGENT_API_CONTROL_TOKEN"
          spellCheck={false}
          type="password"
          value={controlToken}
        />
        {error && <div className="connection-error" role="alert">{error}</div>}
      </form>
    </main>
  );
}
