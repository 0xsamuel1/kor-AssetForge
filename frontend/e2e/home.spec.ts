import { test, expect } from "@playwright/test";

test.describe("Home page", () => {
  test("loads and shows the site header", async ({ page }) => {
    await page.goto("/");
    await expect(page).toHaveTitle(/AssetForge/i);
    await expect(page.locator("header")).toBeVisible();
  });

  test("navigates to the create-asset flow", async ({ page }) => {
    await page.goto("/");
    const createLink = page.getByRole("link", { name: /create/i }).first();
    if (await createLink.count()) {
      await createLink.click();
      await expect(page).toHaveURL(/create-asset/);
    }
  });
});
