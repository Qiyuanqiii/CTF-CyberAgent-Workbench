import type { components } from "./schema";

export type APIErrorView = components["schemas"]["APIError"];
export type ArtifactView = components["schemas"]["ArtifactView"];
export type EventView = components["schemas"]["EventView"];
export type HealthView = components["schemas"]["HealthView"];
export type MessageView = components["schemas"]["MessageView"];
export type NoteView = components["schemas"]["NoteView"];
export type Page = components["schemas"]["Page"];
export type RunDetailView = components["schemas"]["RunDetailView"];
export type RunEventStreamView = components["schemas"]["RunEventStreamView"];
export type RunView = components["schemas"]["RunView"];
export type SessionDetailView = components["schemas"]["SessionDetailView"];
export type SessionView = components["schemas"]["SessionView"];
export type SupervisorToolRoundView = components["schemas"]["SupervisorToolRoundView"];
export type WorkItemView = components["schemas"]["WorkItemView"];

export interface SuccessEnvelope<T> {
  version: "api.v1";
  request_id: string;
  data: T;
  page?: Page;
}

export interface ErrorEnvelope {
  version: "api.v1";
  request_id: string;
  error: APIErrorView;
}

export interface PageResult<T> {
  items: T[];
  page: Page;
  requestID: string;
}
