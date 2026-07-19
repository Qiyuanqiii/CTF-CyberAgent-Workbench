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
  providerCredentialEnabled: boolean;
  fileEditReviewEnabled: boolean;
  fileEditProposalEnabled: boolean;
  fileEditApplyEnabled: boolean;
  runWakeControlEnabled: boolean;
  runWakeExecutionEnabled: boolean;
  runWakeWorkerEnabled: boolean;
  skillInstallationEnabled: boolean;
  evidenceAttachmentEnabled: boolean;
  verificationEvidenceEnabled: boolean;
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
  providerCredentialEnabled: false,
  fileEditReviewEnabled: false,
  fileEditProposalEnabled: false,
  fileEditApplyEnabled: false,
  runWakeControlEnabled: false,
  runWakeExecutionEnabled: false,
  runWakeWorkerEnabled: false,
  skillInstallationEnabled: false,
  evidenceAttachmentEnabled: false,
  verificationEvidenceEnabled: false,
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
	  providerCredentialEnabled: present && (capabilities.providerCredentialEnabled ?? false),
	  fileEditReviewEnabled: present && (capabilities.fileEditReviewEnabled ?? true),
	  fileEditProposalEnabled: present && (capabilities.fileEditProposalEnabled ?? false),
	  fileEditApplyEnabled: present && (capabilities.fileEditApplyEnabled ?? true),
	  runWakeControlEnabled: present && (capabilities.runWakeControlEnabled ?? true),
	  runWakeExecutionEnabled: present && (capabilities.runWakeExecutionEnabled ?? true),
	  runWakeWorkerEnabled: present && (capabilities.runWakeWorkerEnabled ?? false),
	  skillInstallationEnabled: present && (capabilities.skillInstallationEnabled ?? true),
	  evidenceAttachmentEnabled: present && (capabilities.evidenceAttachmentEnabled ?? true),
	  verificationEvidenceEnabled: present &&
	    (capabilities.verificationEvidenceEnabled ?? false),
    });
  },
  disconnect: () => set({ token: "", controlToken: "", health: null,
    runControlEnabled: false, runCreationEnabled: false, sessionMessageEnabled: false,
    sessionSteeringControlEnabled: false,
    runLifecycleEnabled: false, runExecutionEnabled: false,
	planDeliveryControlEnabled: false, approvalControlEnabled: false,
	modelControlEnabled: false, providerCredentialEnabled: false,
	fileEditReviewEnabled: false, fileEditProposalEnabled: false, fileEditApplyEnabled: false,
	runWakeControlEnabled: false, runWakeExecutionEnabled: false, runWakeWorkerEnabled: false,
	skillInstallationEnabled: false,
	evidenceAttachmentEnabled: false,
	verificationEvidenceEnabled: false,
    ...initialSelection }),
  selectRun: (selectedRunID) => set({ selectedRunID, resourceKind: "run" }),
  selectSession: (selectedSessionID) => set({ selectedSessionID, resourceKind: "session" }),
  setHealth: (health) => set({ health }),
  setResourceKind: (resourceKind) => set({ resourceKind }),
}));
