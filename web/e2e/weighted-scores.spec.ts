import { test, expect } from "@playwright/test";
import { mockAllAPIs } from "./helpers/api-mock";

test.describe("Weighted Scores", () => {
  test("unified runs table shows pass rate for completed runs", async ({ page }) => {
    await mockAllAPIs(page);
    await page.goto("/");

    // The unified table shows Pass Rate instead of W. Score
    await expect(page.locator("th", { hasText: "Pass Rate" })).toBeVisible();

    // run-001 has passCount 4, taskCount 4 → "100%"
    const rows = page.locator("tbody tr");
    await expect(rows.first().getByText("100%")).toBeVisible();
  });

  test("unified runs table shows status column", async ({ page }) => {
    await mockAllAPIs(page);
    await page.goto("/");

    // The unified table has a Status column with badges
    await expect(page.locator("th", { hasText: "Status" })).toBeVisible();

    // All mock runs are completed → "Complete" badges
    const rows = page.locator("tbody tr");
    await expect(rows.first().getByText("Complete")).toBeVisible();
  });

  test("run detail task table shows W. Score column", async ({ page }) => {
    await mockAllAPIs(page);
    await page.goto("/#/runs/run-001");

    await expect(page.getByText("W. Score")).toBeVisible();

    // explain-fibonacci has weightedScore 1.0 → "100%"
    // explain-binary-search has weightedScore 0.33 → "33%"
    await expect(page.getByText("explain-fibonacci")).toBeVisible();
    await expect(page.getByText("33%")).toBeVisible();
  });

  test("grader expansion shows weight per grader", async ({ page }) => {
    await mockAllAPIs(page);
    await page.goto("/#/runs/run-001");

    // Expand explain-fibonacci to see grader results
    await page.getByText("explain-fibonacci").click();

    // output-exists has weight 1.0 → "×1"
    await expect(page.getByText("×1")).toBeVisible();
    // mentions-recursion has weight 2.0 → "×2"
    await expect(page.getByText("×2")).toBeVisible();
  });
});
