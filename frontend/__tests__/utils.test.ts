import {
  cn,
  formatAmount,
  formatCurrency,
  truncateAddress,
  formatDate,
} from "@/lib/utils";

describe("cn", () => {
  it("merges class names", () => {
    expect(cn("p-2", "p-4")).toBe("p-4");
  });

  it("ignores falsy values", () => {
    expect(cn("text-sm", false && "hidden", undefined, "font-bold")).toBe(
      "text-sm font-bold",
    );
  });
});

describe("formatAmount", () => {
  it("formats numbers with grouping", () => {
    expect(formatAmount(1234567.89)).toBe("1,234,567.89");
  });

  it("accepts string input", () => {
    expect(formatAmount("42")).toBe("42");
  });
});

describe("formatCurrency", () => {
  it("formats USD by default", () => {
    expect(formatCurrency(1234.5)).toBe("$1,234.50");
  });
});

describe("truncateAddress", () => {
  it("truncates a long address", () => {
    expect(truncateAddress("GABCDEFGHIJKLMNOPQRSTUVWXYZ123456")).toBe(
      "GABCDE...3456",
    );
  });

  it("returns empty string for empty input", () => {
    expect(truncateAddress("")).toBe("");
  });
});

describe("formatDate", () => {
  it("formats a unix timestamp", () => {
    const result = formatDate(0);
    expect(result).toContain("1970");
  });
});
