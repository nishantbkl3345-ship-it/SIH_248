import { expect, test, type Page } from "@playwright/test";

// The acceptance path for the scenario builder, driven through the real UI:
// create -> add phases -> add information -> add degradation -> add a decision
// point -> save -> reopen -> edit -> preview -> publish.

const adminEmail = process.env.E2E_ADMIN_EMAIL;
const adminPassword = process.env.E2E_ADMIN_PASSWORD;

const instructor = {
  email: `instructor-${Date.now()}@e2e.example.test`,
  password: "e2e-password-123",
  displayName: "E2E Instructor",
};

test.skip(!adminEmail || !adminPassword, "Set E2E_ADMIN_EMAIL and E2E_ADMIN_PASSWORD to run end-to-end tests");

test.beforeAll(async ({ playwright, baseURL }) => {
  const api = await playwright.request.newContext({ baseURL });
  const login = await api.post("/api/v1/auth/login", { data: { email: adminEmail, password: adminPassword } });
  expect(login.ok(), "admin login").toBeTruthy();
  const created = await api.post("/api/v1/admin/users", { data: { ...instructor, role: "INSTRUCTOR" } });
  expect(created.ok(), "create instructor").toBeTruthy();
  await api.dispose();
});

const saved = (page: Page) => expect(page.getByRole("status").getByText("All changes saved")).toBeVisible({ timeout: 15_000 });

/** Switches tab and waits until the previous panel has been replaced. */
async function openTab(page: Page, name: string | RegExp) {
  await page.getByRole("tab", { name }).click();
  await expect(page.getByRole("tab", { name })).toHaveAttribute("aria-selected", "true");
  await expect(page.getByRole("tabpanel")).toHaveCount(1);
}

async function addEvent(page: Page, name: string) {
  await page.getByRole("button", { name: "Add event" }).click();
  await page.getByRole("menuitem", { name }).click();
  const panel = page.getByRole("dialog");
  await expect(panel).toBeVisible();
  return panel;
}

async function done(page: Page) {
  await page.getByRole("dialog").getByRole("button", { name: "Done" }).click();
  await expect(page.getByRole("dialog")).toBeHidden();
}

test("an instructor builds, reopens, edits, previews and publishes a scenario", async ({ page }) => {
  const title = `Exercise Tidewatch ${Date.now()}`;
  // Only the selected tab's panel is exposed; scoping to it keeps lookups unambiguous.
  const tab = page.getByRole("tabpanel");

  await test.step("sign in", async () => {
    await page.goto("/login");
    await page.getByLabel("Email").fill(instructor.email);
    await page.getByLabel("Password").fill(instructor.password);
    await page.getByRole("button", { name: "Sign in" }).click();
    await expect(page).toHaveURL(/\/instructor$/);
  });

  await test.step("create scenario", async () => {
    await page.getByRole("link", { name: "Scenarios", exact: true }).click();
    await expect(page.getByRole("heading", { name: "No scenarios yet" })).toBeVisible();
    await page.getByRole("button", { name: "New scenario" }).click();
    await page.getByLabel("Scenario name").fill(title);
    await page.getByRole("button", { name: "Create and open" }).click();
    await expect(page).toHaveURL(/\/instructor\/scenarios\/[0-9a-f-]{36}$/);
    await expect(page.getByRole("heading", { level: 1, name: title })).toBeVisible();
    await expect(page.getByText("Draft", { exact: true })).toBeVisible();
    // A new draft is incomplete, so it cannot be published yet.
    await expect(page.getByRole("button", { name: "Publish" })).toBeDisabled();
  });

  await test.step("fill in metadata", async () => {
    await openTab(page, "Setup");
    await tab
      .getByLabel("Description")
      .fill("A storm has cut the fictional island of Veridia in two. Teams route a relief convoy.");
    await tab.getByLabel("Difficulty").selectOption("ADVANCED");
    await tab.getByLabel("Teams").fill("3");
    await tab.getByLabel("Trainees").fill("12");
    await page.getByRole("button", { name: "Add objective" }).click();
    await tab.getByLabel("Objective 1").fill("Decide with incomplete and conflicting information");
    await expect(tab.getByLabel("Channel 1 name")).toHaveValue("Team Net");
  });

  await test.step("add phases", async () => {
    await openTab(page, "Timeline");
    await tab.getByLabel("Phase title").fill("Assessment");
    await tab.getByLabel("Duration").fill("6:00");
    await page.getByRole("button", { name: "Add phase" }).click();
    await tab.getByLabel("Phase title").fill("Commitment");
    await tab.getByLabel("Duration").fill("6:00");
    await expect(tab.getByText("06:00–12:00").first()).toBeVisible();
    // Back to the first phase.
    await page.getByRole("button", { name: /Assessment.*00:00–06:00/ }).click();
    await expect(tab.getByLabel("Phase title")).toHaveValue("Assessment");
  });

  await test.step("add information", async () => {
    const panel = await addEvent(page, "New synthetic report appears");
    await panel.getByLabel("Title").fill("Report A");
    await panel.getByLabel("Source", { exact: true }).fill("Relay Station K-7");
    await panel.getByLabel("Arrives at").fill("2:00");
    await panel.getByLabel("Message").fill("Coastal road passable as far as the river crossing.");
    await panel.getByLabel("Priority").selectOption("HIGH");
    await panel.getByLabel("Target team").selectOption("2");
    await expect(panel.getByLabel("Delivery channel")).not.toHaveValue("__all__");
    await done(page);
    await expect(tab.getByRole("list").getByRole("button", { name: /Report A arrives/ })).toBeVisible();
  });

  await test.step("add degradation events", async () => {
    let panel = await addEvent(page, "Channel becomes degraded");
    await panel.getByLabel("Start time").fill("4:00");
    await panel.getByLabel("Content delivered (%)").fill("40");
    await done(page);

    panel = await addEvent(page, "Information becomes delayed");
    await panel.getByLabel("Start time").fill("5:00");
    await panel.getByLabel("Duration", { exact: true }).fill("1:00");
    await panel.getByLabel("Delay duration").fill("0:45");
    await done(page);

    panel = await addEvent(page, "Information becomes unavailable");
    await panel.getByLabel("Start time").fill("5:30");
    await panel.getByLabel("Duration", { exact: true }).fill("0:20");
    await panel.getByLabel("Affected team").selectOption("1");
    await done(page);

    // Second phase: conflicting reports.
    await page.getByRole("button", { name: /Commitment.*06:00–12:00/ }).click();
    panel = await addEvent(page, "Conflicting reports appear");
    await panel.getByLabel("Reports arrive at").fill("1:00");
    await panel.getByLabel("Subject").fill("Inland pass");
    await panel.getByLabel("Source A", { exact: true }).fill("Relay Station K-7");
    await panel.getByLabel("Source A reports").fill("The inland pass is open.");
    await panel.getByLabel("Source B", { exact: true }).fill("Community Net");
    await panel.getByLabel("Source B reports").fill("The inland pass is blocked by a slide.");
    await done(page);

    await expect(tab.getByRole("list").getByRole("button", { name: /Conflicting reports: Inland pass/ })).toBeVisible();
  });

  await test.step("add a decision point", async () => {
    const panel = await addEvent(page, "Decision point opens and closes");
    await panel.getByLabel("Question").fill("Which route does the convoy take?");
    await panel.getByLabel("Opens at").fill("3:00");
    await panel.getByLabel("Time to decide").fill("2:00");
    await panel.getByLabel("Option A").fill("Coastal road");
    await panel.getByLabel("Option B").fill("Inland pass");
    await panel.getByRole("button", { name: "Add option" }).click();
    await panel.getByLabel("Option C").fill("Hold and request confirmation");
    await expect(panel.getByLabel("Rationale required")).toBeChecked();
    await panel.getByLabel("Evaluation criteria").fill("Sought confirmation of the pass before committing.");
    await done(page);

    await expect(tab.getByText("Decision point opens")).toBeVisible();
    await expect(tab.getByText("Decision point closes")).toBeVisible();
  });

  await test.step("save: autosave completes and the scenario is ready", async () => {
    await saved(page);
    await openTab(page, /Readiness/);
    await expect(tab.getByText("Ready to publish")).toBeVisible();
    await expect(page.getByRole("button", { name: "Publish" })).toBeEnabled();
  });

  await test.step("reopen: everything is still there after a reload", async () => {
    await page.reload();
    await expect(page.getByRole("heading", { level: 1, name: title })).toBeVisible();
    await expect(tab.getByLabel("Phase title")).toHaveValue("Assessment");
    await expect(tab.getByRole("list").getByRole("button", { name: /Report A arrives/ })).toBeVisible();
    await expect(tab.getByRole("list").getByRole("button", { name: /Information delayed by 45 s/ })).toBeVisible();
    await expect(tab.getByRole("list").getByRole("button", { name: /Information unavailable/ })).toBeVisible();
    await expect(tab.getByRole("list").getByRole("button", { name: /Channel degraded: 40%/ })).toBeVisible();

    await openTab(page, "Setup");
    await expect(tab.getByLabel("Difficulty")).toHaveValue("ADVANCED");
    await expect(tab.getByLabel("Teams")).toHaveValue("3");
    await expect(tab.getByLabel("Objective 1")).toHaveValue("Decide with incomplete and conflicting information");
  });

  await test.step("edit: change a report, and see an introduced problem block publishing", async () => {
    await openTab(page, "Timeline");
    await tab.getByRole("list").getByRole("button", { name: /Report A arrives/ }).click();
    const panel = page.getByRole("dialog");
    await panel.getByLabel("Title").fill("Report A1");
    await panel.getByLabel("Message").fill("");
    await done(page);
    await saved(page);
    await expect(page.getByRole("button", { name: "Publish" })).toBeDisabled();

    // Readiness points at the problem and takes the instructor to it.
    await openTab(page, /Readiness/);
    await page.getByRole("button", { name: /Report has no content/ }).click();
    await expect(panel.getByText("Report has no content.")).toBeVisible();
    await panel.getByLabel("Message").fill("Coastal road passable to the river crossing; bridge unverified.");
    await done(page);
    await saved(page);
    await expect(page.getByRole("button", { name: "Publish" })).toBeEnabled();
  });

  await test.step("preview: the run sheet shows the whole scenario in order", async () => {
    await openTab(page, "Preview");
    const rows = await tab.locator("ol > li").filter({ has: page.locator("time") }).allInnerTexts();
    const timeline = rows.map((r) => r.split("\n").slice(0, 2).join(" "));
    expect(timeline).toEqual([
      "00:00 Exercise begins",
      "02:00 Report A1 arrives",
      "04:00 Channel degraded: 40% of content gets through",
      "05:00 Channel restored",
      "05:00 Information delayed by 45 s",
      "05:30 Information unavailable",
      "05:50 Information available again",
      "06:00 Delay ends",
      "06:00 Phase transition: Commitment",
      "07:00 Conflicting reports: Inland pass",
      "09:00 Decision point opens",
      "11:00 Decision point closes",
      "12:00 Exercise ends",
    ]);
    await expect(tab.getByText("The inland pass is blocked by a slide.")).toBeVisible();
    await expect(tab.getByText("Hold and request confirmation")).toBeVisible();

    // Team 2 is not subject to the dropout aimed at Team 1.
    await page.getByRole("button", { name: "Team 2" }).click();
    await expect(tab.getByText("Information unavailable")).toBeHidden();
    await expect(tab.getByText("Report A1 arrives")).toBeVisible();
    // Team 1 does not receive the report addressed to Team 2.
    await page.getByRole("button", { name: "Team 1" }).click();
    await expect(tab.getByText("Information unavailable")).toBeVisible();
    await expect(tab.getByText("Report A1 arrives")).toBeHidden();
  });

  await test.step("publish", async () => {
    await page.getByRole("button", { name: "Publish" }).click();
    const dialog = page.getByRole("dialog");
    const confirm = dialog.getByRole("button", { name: "Publish scenario" });
    await expect(confirm).toBeDisabled();
    await dialog.getByRole("checkbox").check();
    await confirm.click();
    await expect(page.getByText("Published and locked")).toBeVisible();
    await expect(page.getByRole("button", { name: "Revert to draft" })).toBeVisible();
    await expect(page.getByRole("tab", { name: "Setup" })).toHaveCount(0);

    await page.getByRole("link", { name: "Scenario library" }).click();
    const card = page.getByRole("listitem").filter({ hasText: title });
    await expect(card.getByText("Published")).toBeVisible();
    await expect(card.getByText("12 min")).toBeVisible();
    await expect(card.getByText("2 phases")).toBeVisible();
  });
});
