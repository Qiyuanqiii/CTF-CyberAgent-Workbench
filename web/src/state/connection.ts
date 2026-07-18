import { create } from "zustand";
import type { ClientCapabilities } from "../api/client";
import type { HealthView } from "../api/types";

type ResourceKind = "run" | "session";

interface ConnectionState {
  health: HealthView | null;
  resourceKind: ResourceKind;
  selectedRunID: string;
  selectedSessionID: string;
  token: string;
  controlToken: string;
  runControlEnabled: boolean;
  runCreationEnabled: boolean;
  sessionMessageEnabled: boolean;
  sessionSteeringControlEnabled: boolean;
  runLifecycleEnabled: boolean;
  runExecutionEnabled: boolean;
  planDeliveryControlEnabled: boolean;
  approvalControlEnabled: boolean;
  modelControlEnabled: boolean;
  fileEditReviewEnabled: boolean;
  fileEditApplyEnabled: boolean;
  runWakeControlEnabled: boolean;
  runWakeExecutionEnabled: boolean;
  skillInstallationEnabled: boolean;
  connect: (token: string, health: HealthView, controlToken?: string,
    capabilities?: ClientCapabilities) => void;
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
  controlToken: "",
  runControlEnabled: false,
  runCreationEnabled: false,
  sessionMessageEnabled: false,
  sessionSteeringControlEnabled: false,
  runLifecycleEnabled: false,
  runExecutionEnabled: false,
  planDeliveryControlEnabled: false,
  approvalControlEnabled: false,
  modelControlEnabled: false,
  fileEditReviewEnabled: false,
  fileEditApplyEnabled: false,
  runWakeControlEnabled: false,
  runWakeExecutionEnabled: false,
  skillInstallationEnabled: false,
  connect: (token, health, controlToken = "", capabilities = {}) => {
    const present = controlToken.trim() !== "";
    set({ token, health, controlToken,
      runControlEnabled: present && (capabilities.runControlEnabled ?? true),
      runCreationEnabled: present && (capabilities.runCreationEnabled ?? true),
      sessionMessageEnabled: present && (capabilities.sessionMessageEnabled ?? true),
      sessionSteeringControlEnabled: present &&
        (capabilities.sessionSteeringControlEnabled ?? true),
      runLifecycleEnabled: present && (capabilities.runLifecycleEnabled ?? true),
      runExecutionEnabled: present && (capabilities.runExecutionEnabled ?? true),
      planDeliveryControlEnabled: present &&
        (capabilities.planDeliveryControlEnabled ?? true),
      approvalControlEnabled: present && (capabilities.approvalControlEnabled ?? true),
	  modelControlEnabled: present && (capabilities.modelControlEnabled ?? true),
	  fileEditReviewEnabled: present && (capabilities.fileEditReviewEnabled ?? true),
	  fileEditApplyEnabled: present && (capabilities.fileEditApplyEnabled ?? true),
	  runWakeControlEnabled: present && (capabilities.runWakeControlEnabled ?? true),
	  runWakeExecutionEnabled: present && (capabilities.runWakeExecutionEnabled ?? true),
	  skillInstallationEnabled: present && (capabilities.skillInstallationEnabled ?? true),
    });
  },
  disconnect: () => set({ token: "", controlToken: "", health: null,
    runControlEnabled: false, runCreationEnabled: false, sessionMessageEnabled: false,
    sessionSteeringControlEnabled: false,
    runLifecycleEnabled: false, runExecutionEnabled: false,
	planDeliveryControlEnabled: false, approvalControlEnabled: false,
	modelControlEnabled: false, fileEditReviewEnabled: false, fileEditApplyEnabled: false,
	runWakeControlEnabled: false, runWakeExecutionEnabled: false, skillInstallationEnabled: false,
    ...initialSelection }),
  selectRun: (selectedRunID) => set({ selectedRunID, resourceKind: "run" }),
  selectSession: (selectedSessionID) => set({ selectedSessionID, resourceKind: "session" }),
  setHealth: (health) => set({ health }),
  setResourceKind: (resourceKind) => set({ resourceKind }),
}));
