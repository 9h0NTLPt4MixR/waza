import { test, expect } from "@playwright/test";
import { mockAllAPIs, mockEmptyAPIs } from "./helpers/api-mock";
import { SUMMARY, RUNS } from "./fixtures/mock-data";

test.describe("Dashboard", () => {
  test("page loads and shows waza branding in nav", async ({ page }) => {
    await mockAllAPIs(page);
    await page.goto("/");

    const header = page.locator("header");
    await expect(header.getByText("waza")).toBeVisible();
    await expect(header.getByText("eval dashboard")).toBeVisible();
  });

  test("6 KPI cards render with data", async ({ page }) => {
    await mockAllAPIs(page);
    await page.goto("/");

    // KPI cards are inside the grid before the table — scope to span labels
    // These label texts only appear in the KPI card <span> elements
    const cards = page.locator("div.grid").first();
    await expect(cards.getByText("Total Runs")).toBeVisible();
    await expect(cards.getByText("Total Tasks")).toBeVisible();
    await expect(cards.getByText("Pass Rate")).toBeVisible();
    await expect(cards.getByText("Avg Tokens")).toBeVisible();
    await expect(cards.getByText("Avg Cost")).toBeVisible();
    await expect(cards.getByText("Avg Duration")).toBeVisible();
  });

  test("KPI card values are properly formatted", async ({ page }) => {
    await mockAllAPIs(page);
    await page.goto("/");

    // Scope to the KPI grid to avoid matching table content
    const cards = page.locator("div.grid").first();

    // totalRuns: 12
    await expect(cards.getByText("12")).toBeVisible();
    // totalTasks: 48
    await expect(cards.getByText("48")).toBeVisible();
    // passRate: 85 → "85%"
    await expect(cards.getByText("85%")).toBeVisible();
    // avgTokens: 15230 → "15.2K"
    await expect(cards.getByText("15.2K")).toBeVisible();
    // avgCost: 1.47 → "$1.47"
    await expect(cards.getByText("$1.47")).toBeVisible();
    // avgDuration: 42s → "42s"
    await expect(cards.getByText("42s")).toBeVisible();
  });

  test("pass rate card shows green for ≥80%", async ({ page }) => {
    await mockAllAPIs(page);
    await page.goto("/");

    // passRate is 85% which should be green-500
    const passRateValue = page.locator("text=85%");
    await expect(passRateValue).toBeVisible();
    await expect(passRateValue).toHaveClass(/text-green-500/);
  });
});

test.describe("Runs Table", () => {
  test("table renders with run data", async ({ page }) => {
    await mockAllAPIs(page);
    await page.goto("/");

    // Check table headers
    await expect(page.getByRole("button", { name: /Spec/ })).toBeVisible();
    await expect(page.getByRole("button", { name: /Model/ })).toBeVisible();

    // Check run data appears
    await expect(page.getByText("code-explainer")).toBeVisible();
    await expect(page.getByText("skill-checker")).toBeVisible();
    await expect(page.getByText("doc-writer")).toBeVisible();
  });

  test("column sorting works", async ({ page }) => {
    await mockAllAPIs(page);
    await page.goto("/");

    // Wait for table to render
    await expect(page.getByText("code-explainer")).toBeVisible();

    // Click the Spec header to trigger sort (asc)
    await page.getByRole("button", { name: /Spec/ }).click();

    // After ascending sort: code-explainer, doc-writer, skill-checker
    // First column is Status badge, spec is the second column
    const rows = page.locator("tbody tr");
    await expect(rows).toHaveCount(3);
    await expect(rows.nth(0).locator("td:nth-child(2)")).toContainText("code-explainer");
    await expect(rows.nth(2).locator("td:nth-child(2)")).toContainText("skill-checker");

    // Click again to reverse (desc)
    await page.getByRole("button", { name: /Spec/ }).click();
    await expect(rows.nth(0).locator("td:nth-child(2)")).toContainText("skill-checker");
  });

  test("status badges display correctly", async ({ page }) => {
    await mockAllAPIs(page);
    await page.goto("/");

    // The unified table uses StatusBadge with emoji text for run status
    // All mock runs are completed, so they should show "✅ Complete"
    const completeBadges = page.locator("tbody").getByText("Complete");
    await expect(completeBadges.first()).toBeVisible();

    // There should be 3 complete badges (one per mock run)
    await expect(completeBadges).toHaveCount(3);
  });

  test("empty state message when no runs exist", async ({ page }) => {
    await mockEmptyAPIs(page);
    await page.goto("/");

    await expect(page.getByText("No runs found.")).toBeVisible();
  });
});
