import { create } from "zustand";
import type { HealthView } from "../api/types";

type ResourceKind = "run" | "session";

interface ConnectionState {
  health: HealthView | null;
  resourceKind: ResourceKind;
  selectedRunID: string;
  selectedSessionID: string;
  token: string;
  connect: (token: string, health: HealthView) => void;
  disconnect: () => void;
  selectRun: (runID: string) => void;
  selectSession: (sessionID: string) => void;
  setHealth: (health: HealthView) => void;
  setResourceKind: (kind: ResourceKind) => void;
}

const initialSelection = {
  resourceKind: "run" as const,
  selectedRunID: "",
  selectedSessionID: "",
};

export const useConnectionStore = create<ConnectionState>((set) => ({
  ...initialSelection,
  health: null,
  token: "",
  connect: (token, health) => set({ token, health }),
  disconnect: () => set({ token: "", health: null, ...initialSelection }),
  selectRun: (selectedRunID) => set({ selectedRunID, resourceKind: "run" }),
  selectSession: (selectedSessionID) => set({ selectedSessionID, resourceKind: "session" }),
  setHealth: (health) => set({ health }),
  setResourceKind: (resourceKind) => set({ resourceKind }),
}));
