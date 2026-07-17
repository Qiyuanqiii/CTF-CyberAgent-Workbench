import { useConnectionStore } from "./connection";

const health = {
  status: "ok" as const,
  api_version: "api.v1" as const,
  app_version: "test",
  schema_version: 37,
};

describe("connection store", () => {
  beforeEach(() => {
    localStorage.clear();
    sessionStorage.clear();
    useConnectionStore.getState().disconnect();
  });

  it("keeps both capability tokens in memory and clears them on disconnect", () => {
    useConnectionStore.getState().connect("ephemeral-token", health, "ephemeral-control-token");
    useConnectionStore.getState().selectRun("run-1");

    expect(useConnectionStore.getState().token).toBe("ephemeral-token");
    expect(useConnectionStore.getState().controlToken).toBe("ephemeral-control-token");
   expect(useConnectionStore.getState().runControlEnabled).toBe(true);
   expect(useConnectionStore.getState().runCreationEnabled).toBe(true);
    expect(localStorage.length).toBe(0);
    expect(sessionStorage.length).toBe(0);

    useConnectionStore.getState().disconnect();
    expect(useConnectionStore.getState().token).toBe("");
    expect(useConnectionStore.getState().controlToken).toBe("");
   expect(useConnectionStore.getState().runControlEnabled).toBe(false);
   expect(useConnectionStore.getState().runCreationEnabled).toBe(false);
    expect(useConnectionStore.getState().selectedRunID).toBe("");
  });
});
