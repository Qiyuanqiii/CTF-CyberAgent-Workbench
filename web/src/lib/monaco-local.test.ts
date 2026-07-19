describe("local Monaco boundary", () => {
  it("pins the editor and every language worker to local bundled modules", () => {
    const modules = import.meta.glob("./monaco-local.ts", {
      eager: true, import: "default", query: "?raw",
    });
    const source = modules["./monaco-local.ts"];
    expect(typeof source).toBe("string");
    if (typeof source !== "string") throw new Error("local Monaco source was not loaded as text");
    expect(source).toContain('from "monaco-editor"');
    expect(source).toContain("loader.config({ monaco })");
    for (const worker of ["editor.worker?worker", "json.worker?worker", "css.worker?worker",
      "html.worker?worker", "ts.worker?worker"]) {
      expect(source).toContain(worker);
    }
    expect(source).not.toContain("https://");
    expect(source).not.toContain("cdn.jsdelivr");
  });
});
