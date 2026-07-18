import { useQuery } from "@tanstack/react-query";
import { Cpu, Route, X } from "lucide-react";
import type { CyberAgentClient } from "../api/client";
import { ErrorState, LoadingState, StatusBadge } from "./common";

export function ModelAvailabilityDialog({ client, open, onClose }: {
  client: CyberAgentClient;
  open: boolean;
  onClose: () => void;
}) {
  const query = useQuery({
    queryKey: ["models", "availability"],
    queryFn: ({ signal }) => client.modelAvailability(signal),
    enabled: open,
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
                    </div>
                  ))}
                </div>
              </section>
              <section className="model-availability-section">
                <h3><Route aria-hidden="true" size={14} />Routes</h3>
                <div className="model-route-list">
                  {query.data.routes.map((route) => (
                    <div className="model-route-row" key={route.name}>
                      <strong>{route.name}</strong>
                      <span>{route.provider}/{route.model}</span>
                      <StatusBadge status={route.available ? "available" : "unavailable"} />
                    </div>
                  ))}
                </div>
              </section>
            </>
          )}
        </div>
      </section>
    </div>
  );
}
