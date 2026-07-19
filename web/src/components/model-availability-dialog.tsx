import { useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, Check, Cpu, KeyRound, LoaderCircle, Route, Save, Trash2, X } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import type { ProviderDiagnosticView } from "../api/types";
import { ErrorState, LoadingState, StatusBadge } from "./common";

export function ModelAvailabilityDialog({ client, open, onClose }: {
  client: CyberAgentClient;
  open: boolean;
  onClose: () => void;
}) {
  const queryClient = useQueryClient();
  const [selections, setSelections] = useState<Record<string, string>>({});
  const [diagnostic, setDiagnostic] = useState<ProviderDiagnosticView | null>(null);
  const [credentialBusy, setCredentialBusy] = useState("");
  const [credentialError, setCredentialError] = useState("");
  const [credentialRestart, setCredentialRestart] = useState(false);
  const credentialInputs = useRef(new Map<string, HTMLInputElement>());
  const query = useQuery({
    queryKey: ["models", "availability"],
    queryFn: ({ signal }) => client.modelAvailability(signal),
    enabled: open,
  });
  const credentialQuery = useQuery({
    queryKey: ["models", "credentials"],
    queryFn: ({ signal }) => client.providerCredentialStatuses(signal),
    enabled: open && client.hasProviderCredentials,
  });
  const routeMutation = useMutation({
    mutationFn: ({ route, reference }: { route: string; reference: string }) => {
      const slash = reference.indexOf("/");
      if (slash <= 0 || slash === reference.length - 1) {
        throw new Error("Select an available Provider model");
      }
      return client.selectModelRoute(route, {
        version: "model_route_control.v1",
        provider: reference.slice(0, slash), model: reference.slice(slash + 1),
      });
    },
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: ["models", "availability"] }),
  });
  const diagnosticMutation = useMutation({
    mutationFn: ({ provider, model }: { provider: string; model: string }) =>
      client.diagnoseProvider({ version: "provider_diagnostic.v1", provider, model,
        confirm_diagnostic: true }),
    onSuccess: setDiagnostic,
  });
  const changeCredential = async (provider: string, action: "set" | "delete") => {
    if (credentialBusy) return;
    const input = credentialInputs.current.get(provider);
    const secret = action === "set" ? input?.value ?? "" : "";
    if (action === "set" && secret.length < 8) {
      setCredentialError("Credential must contain at least 8 non-space characters");
      return;
    }
    if (input) input.value = "";
    setCredentialBusy(provider);
    setCredentialError("");
    const body = { version: "provider_credential.v1" as const, action,
      secret, confirm: true };
    try {
      const status = await client.changeProviderCredential(provider, body);
      setCredentialRestart(status.restart_required);
      await Promise.all([credentialQuery.refetch(),
        queryClient.invalidateQueries({ queryKey: ["models", "availability"] })]);
    } catch (caught) {
      setCredentialError(caught instanceof Error ? caught.message : "Credential change failed");
    } finally {
      body.secret = "";
      setCredentialBusy("");
    }
  };
  if (!open) {
    return null;
  }
  return (
    <div className="desktop-dialog-backdrop" role="presentation">
      <section aria-label="Model availability" aria-modal="true"
        className="desktop-dialog model-availability-dialog" role="dialog">
        <header>
          <div>
            <span className="dialog-icon"><Cpu aria-hidden="true" size={18} /></span>
            <div><h2>Models / 模型</h2><small>model_availability.v1</small></div>
          </div>
          <button aria-label="Close model availability" className="icon-button"
            onClick={onClose} title="Close" type="button">
            <X aria-hidden="true" size={17} />
          </button>
        </header>
        <div className="desktop-dialog-body model-availability-body">
          {query.isLoading && <LoadingState label="Loading model availability" />}
          {query.isError && <ErrorState error={query.error} />}
          {query.data && (
            <>
              <section className="model-availability-section">
                <h3><Cpu aria-hidden="true" size={14} />Providers</h3>
                <div className="model-provider-list">
                  {query.data.providers.map((provider) => (
                    <div className="model-provider-row" key={provider.name}>
                      <div><strong>{provider.name}</strong><small>{provider.kind}</small></div>
                      <span>{provider.models.join(", ") || "No configured model"}</span>
                      <span>{provider.credential_source}</span>
                      <StatusBadge status={provider.status} />
                      {client.hasModelControl && provider.status === "available" &&
                        provider.models[0] && (
                          <button aria-label={`Diagnose ${provider.name}`} className="icon-button"
                            disabled={diagnosticMutation.isPending}
                            onClick={() => diagnosticMutation.mutate({ provider: provider.name,
                              model: provider.models[0]! })}
                            title="Run explicit connectivity diagnostic" type="button">
                            {diagnosticMutation.isPending &&
                              diagnosticMutation.variables?.provider === provider.name
                              ? <LoaderCircle aria-hidden="true" className="spin" size={15} />
                              : <Activity aria-hidden="true" size={15} />}
                          </button>
                        )}
                    </div>
                  ))}
                </div>
                {diagnostic && <div className="model-diagnostic-result" role="status">
                  <span>{diagnostic.provider}/{diagnostic.model}</span>
                  <StatusBadge status={diagnostic.status} />
                  <span>{diagnostic.outcome}</span>
                  <span>{diagnostic.duration_ms} ms</span>
                </div>}
                {diagnosticMutation.isError && <div className="inline-warning" role="alert">
                  {diagnosticMutation.error instanceof Error
                    ? diagnosticMutation.error.message : "Provider diagnostic failed"}
                </div>}
              </section>
              {client.hasProviderCredentials && <section className="model-availability-section">
                <h3><KeyRound aria-hidden="true" size={14} />System credentials</h3>
                {credentialQuery.isLoading && <LoadingState label="Loading credential status" />}
                {credentialQuery.isError && <ErrorState error={credentialQuery.error} />}
                {credentialQuery.data && <div className="provider-credential-list">
                  {credentialQuery.data.items.map((item) => <div className="provider-credential-row"
                    key={item.provider}>
                    <div><strong>{item.provider}</strong><small>{item.store_kind}</small></div>
                    <StatusBadge status={item.configured ? "configured" : "not configured"} />
                    <input aria-label={`${item.provider} API credential`} autoCapitalize="none"
                      autoComplete="off" autoCorrect="off"
                      disabled={!item.store_available || credentialBusy === item.provider}
                      maxLength={2560} ref={(element) => {
                        if (element) credentialInputs.current.set(item.provider, element);
                        else credentialInputs.current.delete(item.provider);
                      }} spellCheck={false} type="password" />
                    <button aria-label={`Store ${item.provider} credential`} className="icon-button"
                      disabled={!item.store_available || Boolean(credentialBusy)}
                      onClick={() => void changeCredential(item.provider, "set")}
                      title="Store in the OS credential manager" type="button">
                      {credentialBusy === item.provider ?
                        <LoaderCircle aria-hidden="true" className="spin" size={15} /> :
                        <Save aria-hidden="true" size={15} />}
                    </button>
                    <button aria-label={`Delete ${item.provider} credential`} className="icon-button"
                      disabled={!item.store_available || !item.configured || Boolean(credentialBusy)}
                      onClick={() => void changeCredential(item.provider, "delete")}
                      title="Delete OS credential" type="button">
                      <Trash2 aria-hidden="true" size={15} />
                    </button>
                  </div>)}
                </div>}
                {credentialRestart && <div className="model-diagnostic-result" role="status">
                  <Check aria-hidden="true" size={14} />Credential status updated
                  <span>Restart required to load the Provider</span>
                </div>}
                {credentialError && <div className="inline-warning" role="alert">
                  {credentialError}
                </div>}
              </section>}
              <section className="model-availability-section">
                <h3><Route aria-hidden="true" size={14} />Routes</h3>
                <div className="model-route-list">
                  {query.data.routes.map((route) => (
                    <div className="model-route-row" key={route.name}>
                      <strong>{route.name}</strong>
                      {client.hasModelControl ? <select aria-label={`${route.name} model route`}
                        onChange={(event) => setSelections((current) => ({ ...current,
                          [route.name]: event.target.value }))}
                        value={selections[route.name] ?? `${route.provider}/${route.model}`}>
                        {query.data.providers.filter((provider) => provider.status === "available")
                          .flatMap((provider) => provider.models.map((model) => (
                            <option key={`${provider.name}/${model}`} value={`${provider.name}/${model}`}>
                              {provider.name}/{model}
                            </option>
                          )))}
                      </select> : <span>{route.provider}/{route.model}</span>}
                      <StatusBadge status={route.available ? "available" : "unavailable"} />
                      {client.hasModelControl && <button aria-label={`Save ${route.name} route`}
                        className="icon-button" disabled={routeMutation.isPending}
                        onClick={() => routeMutation.mutate({ route: route.name,
                          reference: selections[route.name] ?? `${route.provider}/${route.model}` })}
                        title="Persist route selection" type="button">
                        {routeMutation.isPending && routeMutation.variables?.route === route.name
                          ? <LoaderCircle aria-hidden="true" className="spin" size={15} />
                          : <Check aria-hidden="true" size={15} />}
                      </button>}
                    </div>
                  ))}
                </div>
              </section>
              {routeMutation.isError && <div className="inline-warning" role="alert">
                {routeMutation.error instanceof Error
                  ? routeMutation.error.message : "Model route selection failed"}
              </div>}
            </>
          )}
        </div>
      </section>
    </div>
  );
}
