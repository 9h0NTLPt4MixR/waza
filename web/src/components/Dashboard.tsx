import { useMemo } from "react";
import { Download } from "lucide-react";
import KPICards, { KPICardsSkeleton } from "./KPICards";
import RunsTable, { RunsTableSkeleton } from "./RunsTable";
import { useSummary, useRuns, useResults } from "../hooks/useApi";
import { exportRunsToCSV } from "../lib/export";
import type { RunSummary } from "../api/client";

function ErrorBox({
  message,
  onRetry,
}: {
  message: string;
  onRetry: () => void;
}) {
  return (
    <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-6 text-center">
      <p className="text-red-400">{message}</p>
      <button
        onClick={onRetry}
        className="mt-3 rounded bg-zinc-700 px-4 py-2 text-sm text-zinc-100"
      >
        Retry
      </button>
    </div>
  );
}

export default function Dashboard() {
  const summary = useSummary();
  const runs = useRuns();
  const results = useResults();

  // Merge local runs + Cosmos results, dedup by ID (Cosmos wins on conflict)
  const mergedRuns = useMemo<RunSummary[]>(() => {
    const localRuns = runs.data ?? [];
    const cosmosResults = results.data ?? [];

    const byId = new Map<string, RunSummary>();
    for (const run of localRuns) {
      byId.set(run.id, run);
    }
    for (const result of cosmosResults) {
      const key = result.runId ?? result.id;
      const existing = byId.get(key);
      // Cosmos result takes precedence — it's the authoritative completed result
      if (!existing) {
        byId.set(key, {
          id: key,
          spec: result.spec,
          model: result.model,
          judgeModel: result.judgeModel,
          outcome: result.outcome,
          passCount: result.passCount,
          taskCount: result.taskCount,
          tokens: result.tokens,
          cost: result.cost,
          duration: result.duration,
          timestamp: result.timestamp,
          weightedScore: result.weightedScore,
        });
      }
    }

    return Array.from(byId.values()).sort(
      (a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime(),
    );
  }, [runs.data, results.data]);

  const isLoading = runs.isLoading && results.isLoading;
  const hasData = mergedRuns.length > 0;

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-zinc-100">Eval Runs</h1>
        {hasData && (
          <button
            onClick={() => exportRunsToCSV(mergedRuns)}
            className="inline-flex items-center gap-1.5 rounded bg-zinc-700 px-3 py-1.5 text-sm text-zinc-100 hover:bg-zinc-600 transition-colors"
          >
            <Download className="h-3.5 w-3.5" />
            Export CSV
          </button>
        )}
      </div>

      {summary.isLoading && <KPICardsSkeleton />}
      {summary.isError && (
        <ErrorBox
          message={
            summary.error instanceof Error
              ? summary.error.message
              : "Failed to load summary"
          }
          onRetry={() => void summary.refetch()}
        />
      )}
      {summary.data && <KPICards data={summary.data} />}

      {isLoading && <RunsTableSkeleton />}
      {runs.isError && results.isError && (
        <ErrorBox
          message="Failed to load runs"
          onRetry={() => {
            void runs.refetch();
            void results.refetch();
          }}
        />
      )}
      {!isLoading && <RunsTable data={mergedRuns} />}
    </div>
  );
}
