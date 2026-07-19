export function downloadTextFile(filename: string, mimeType: string, content: string): void {
  if (!filename || /[\\/:*?"<>|\u0000-\u001f]/u.test(filename) || !mimeType || !content) {
    throw new Error("A safe filename, MIME type, and non-empty content are required");
  }
  const objectURL = URL.createObjectURL(new Blob([content], { type: mimeType }));
  try {
    const link = document.createElement("a");
    link.href = objectURL;
    link.download = filename;
	link.rel = "noopener";
	link.click();
  } finally {
	globalThis.setTimeout(() => URL.revokeObjectURL(objectURL), 0);
  }
}
