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

  it("keeps the read token in memory and clears it on disconnect", () => {
    useConnectionStore.getState().connect("ephemeral-token", health);
    useConnectionStore.getState().selectRun("run-1");

    expect(useConnectionStore.getState().token).toBe("ephemeral-token");
    expect(localStorage.length).toBe(0);
    expect(sessionStorage.length).toBe(0);

    useConnectionStore.getState().disconnect();
    expect(useConnectionStore.getState().token).toBe("");
    expect(useConnectionStore.getState().selectedRunID).toBe("");
  });
});
