import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Activity, Check, Cpu, LoaderCircle, Route, X } from "lucide-react";
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
  const query = useQuery({
    queryKey: ["models", "availability"],
    queryFn: ({ signal }) => client.modelAvailability(signal),
    enabled: open,
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
