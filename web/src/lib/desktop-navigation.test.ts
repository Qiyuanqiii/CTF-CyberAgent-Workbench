import { installDesktopNavigationGuard } from "./desktop-navigation";

describe("Desktop renderer navigation guard", () => {
  beforeEach(() => {
    window.go = {
      desktop: {
        DesktopBridge: {
          Bootstrap: async () => ({}),
          SelectSkillPackage: async () => ({}),
          PreviewSkillPackage: async () => ({}),
        },
      },
    };
  });

  afterEach(() => {
    document.body.replaceChildren();
    delete window.go;
  });

  it("blocks external links, form actions, and popup calls while allowing same-origin routes", () => {
    const originalOpen = window.open;
    const remove = installDesktopNavigationGuard();

    const external = document.createElement("a");
    external.href = "https://example.com/automated-notes";
    document.body.append(external);
    expect(external.dispatchEvent(new Event("click", { bubbles: true, cancelable: true }))).toBe(false);

    const internal = document.createElement("a");
    internal.href = "#run-local";
    document.body.append(internal);
    expect(internal.dispatchEvent(new Event("click", { bubbles: true, cancelable: true }))).toBe(true);

    const form = document.createElement("form");
    form.action = "https://example.com/upload";
    document.body.append(form);
    expect(form.dispatchEvent(new SubmitEvent("submit", { bubbles: true, cancelable: true }))).toBe(false);
    expect(window.open("https://example.com")).toBeNull();

    remove();
    expect(window.open).toBe(originalOpen);
  });

  it("does not change ordinary browser navigation without the native bridge", () => {
    delete window.go;
    const remove = installDesktopNavigationGuard();
    const external = document.createElement("a");
    external.href = "https://example.com/ordinary-browser";
    document.body.append(external);
    expect(external.dispatchEvent(new Event("click", { bubbles: true, cancelable: true }))).toBe(true);
    remove();
  });
});
