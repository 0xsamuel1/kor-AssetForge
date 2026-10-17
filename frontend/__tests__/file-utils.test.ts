import {
  generateFileId,
  validateFile,
  validateFiles,
  formatFileSize,
} from "@/lib/file-utils";

function makeFile(name: string, type: string, size: number): File {
  const file = new File(["x"], name, { type });
  Object.defineProperty(file, "size", { value: size });
  return file;
}

describe("generateFileId", () => {
  it("generates unique ids", () => {
    const a = generateFileId();
    const b = generateFileId();
    expect(a).not.toBe(b);
    expect(a).toMatch(/^file_/);
  });
});

describe("validateFile", () => {
  it("accepts an allowed file type within size limit", () => {
    const file = makeFile("photo.png", "image/png", 1024);
    expect(validateFile(file, { allowedTypes: ["image/png"], maxSize: 2048 })).toEqual({
      valid: true,
    });
  });

  it("rejects disallowed file types", () => {
    const file = makeFile("script.exe", "application/x-msdownload", 10);
    const result = validateFile(file, { allowedTypes: ["image/png"] });
    expect(result.valid).toBe(false);
    expect(result.error).toContain("not supported");
  });

  it("rejects files exceeding max size", () => {
    const file = makeFile("big.png", "image/png", 5000);
    const result = validateFile(file, { allowedTypes: ["image/png"], maxSize: 1000 });
    expect(result.valid).toBe(false);
    expect(result.error).toContain("exceeds");
  });

  it("rejects files below min size", () => {
    const file = makeFile("tiny.png", "image/png", 1);
    const result = validateFile(file, {
      allowedTypes: ["image/png"],
      minSize: 100,
    });
    expect(result.valid).toBe(false);
    expect(result.error).toContain("too small");
  });
});

describe("validateFiles", () => {
  it("accepts a batch within the max file count", () => {
    const files = [
      makeFile("a.png", "image/png", 100),
      makeFile("b.png", "image/png", 100),
    ];
    expect(validateFiles(files, { allowedTypes: ["image/png"], maxFiles: 5 })).toEqual({
      valid: true,
    });
  });

  it("rejects a batch exceeding the max file count", () => {
    const files = [
      makeFile("a.png", "image/png", 100),
      makeFile("b.png", "image/png", 100),
    ];
    const result = validateFiles(files, { maxFiles: 1 });
    expect(result.valid).toBe(false);
    expect(result.error).toContain("up to 1");
  });
});

describe("formatFileSize", () => {
  it("formats bytes", () => {
    expect(formatFileSize(0)).toBe("0 Bytes");
    expect(formatFileSize(1024)).toBe("1 KB");
    expect(formatFileSize(1024 * 1024)).toBe("1 MB");
  });
});
