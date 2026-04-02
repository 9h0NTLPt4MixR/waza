### 2026-04-02T17:52: User feedback — Platform UI issues

**By:** Shayne Boyer (via Copilot)

**Issues identified from live ADC run testing:**

1. **Eval Spec missing** — RunStatus and RunQueue pages show empty Eval Spec column. The `evalSpec` field from the queue API isn't displayed.

2. **Tokens showing 0** — Results Summary shows Tokens: 0. The Copilot SDK usage stats aren't being captured/returned from ADC sandbox execution. Need to extract token counts from results.json.

3. **Task failure display** — First task "AVM Fallback When No AZD Pattern" shows ❌ fail with 92% score but all individual graders passed (✅). No failure reason shown. The overall outcome logic may be wrong, or the threshold-based pass/fail isn't surfaced.

4. **Dashboard KPI cards all zeros** — "Total Runs: 0, Total Tasks: 0, Pass Rate: 0%" despite having real data in the table below. The `/api/summary` endpoint still uses the old webapi store, not Cosmos.

5. **Dashboard + Queue should be combined** — The two separate pages (Eval Runs + Run Queue) should be a single unified page showing all data.

6. **Workers not utilized** — Run says 3 workers but only 1 ADC sandbox was created. The platform should create N sandboxes for N workers and distribute tasks across them.

**Why:** User feedback from live testing — captured for team prioritization
