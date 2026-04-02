import { useMemo, useState } from "react";
import {
  useReactTable,
  getCoreRowModel,
  getSortedRowModel,
  flexRender,
  createColumnHelper,
  type SortingState,
} from "@tanstack/react-table";
import {
  Download,
  ArrowUpDown,
  RefreshCw,
  RotateCcw,
  XCircle,
  ExternalLink,
} from "lucide-react";
import KPICards, { KPICardsSkeleton } from "./KPICards";
import { RunsTableSkeleton } from "./RunsTable";
import {
  useSummary,
  useRuns,
  useResults,
  useRunQueue,
  useCancelRun,
  useRerunRun,
} from "../hooks/useApi";
import { exportRunsToCSV } from "../lib/export";
import {
  formatDuration,
  formatRelativeTime,
  formatPercent,
} from "../lib/format";
import type { RunSummary, RunQueueItem } from "../api/client";

/* ── Unified row type ─────────────────────────────────────────────── */

interface UnifiedRun {
  id: string;
  status: "queued" | "running" | "complete" | "failed" | "cancelled";
  spec: string;
  model: string;
  judgeModel?: string;
  passRate: number | null;
  taskCount: number | null;
  duration: number | null;
  timestamp: string;
  error?: string;
  source: "queue" | "results";
}

/* ── Status badge config ──────────────────────────────────────────── */

const STATUS_CONFIG: Record<
  string,
  { emoji: string; label: string; color: string }
> = {
  queued: { emoji: "🟡", label: "Queued", color: "text-yellow-300" },
  running: { emoji: "🔵", label: "Running", color: "text-blue-300" },
  complete: { emoji: "✅", label: "Complete", color: "text-emerald-300" },
  failed: { emoji: "❌", label: "Failed", color: "text-red-300" },
  cancelled: { emoji: "⚪", label: "Cancelled", color: "text-zinc-400" },
};

function StatusBadge({ status }: { status: string }) {
  const cfg = STATUS_CONFIG[status] ?? STATUS_CONFIG.queued!;
  return (
    <span className={`inline-flex items-center gap-1 text-sm ${cfg.color}`}>
      {cfg.emoji} {cfg.label}
    </span>
  );
}

/* ── Helpers ───────────────────────────────────────────────────────── */

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

function mapQueueItem(q: RunQueueItem): UnifiedRun {
  return {
    id: q.id,
    status: q.status,
    spec: q.evalSpec
      ? q.evalSpec.split("/").slice(-2).join("/")
      : "—",
    model: q.model,
    passRate: null,
    taskCount: null,
    duration: null,
    timestamp: q.createdAt,
    error: q.error,
    source: "queue",
  };
}

function mapResultRun(r: RunSummary): UnifiedRun {
  return {
    id: r.id,
    status: "complete",
    spec: r.spec,
    model: r.model,
    judgeModel: r.judgeModel,
    passRate: r.taskCount > 0 ? r.passCount / r.taskCount : null,
    taskCount: r.taskCount,
    duration: r.duration,
    timestamp: r.timestamp,
    source: "results",
  };
}

/* ── Table ─────────────────────────────────────────────────────────── */

const col = createColumnHelper<UnifiedRun>();

function UnifiedRunsTable({
  data,
  onCancel,
  onRerun,
  cancelPending,
  rerunPending,
}: {
  data: UnifiedRun[];
  onCancel: (id: string) => void;
  onRerun: (id: string) => void;
  cancelPending: boolean;
  rerunPending: boolean;
}) {
  const [sorting, setSorting] = useState<SortingState>([]);

  const columns = useMemo(
    () => [
      col.accessor("status", {
        header: "Status",
        cell: (info) => <StatusBadge status={info.getValue()} />,
        size: 110,
        enableSorting: true,
      }),
      col.accessor("spec", {
        header: "Spec",
        cell: (info) => (
          <span className="font-medium text-zinc-100">{info.getValue()}</span>
        ),
      }),
      col.accessor("model", {
        header: "Model",
        cell: (info) => {
          const row = info.row.original;
          return (
            <span className="text-zinc-300">
              {info.getValue()}
              {row.judgeModel && row.judgeModel !== info.getValue() && (
                <span className="ml-1.5 text-xs text-purple-400" title={`Judge: ${row.judgeModel}`}>⚖</span>
              )}
            </span>
          );
        },
      }),
      col.accessor("passRate", {
        header: "Pass Rate",
        cell: (info) => {
          const val = info.getValue();
          return (
            <span className="text-zinc-300">
              {val != null ? formatPercent(val) : "—"}
            </span>
          );
        },
      }),
      col.accessor("taskCount", {
        header: "Tasks",
        cell: (info) => {
          const val = info.getValue();
          return (
            <span className="text-zinc-300">{val != null ? val : "—"}</span>
          );
        },
      }),
      col.accessor("duration", {
        header: "Duration",
        cell: (info) => {
          const val = info.getValue();
          return (
            <span className="text-zinc-300">
              {val != null ? formatDuration(val) : "—"}
            </span>
          );
        },
      }),
      col.accessor("timestamp", {
        header: "When",
        cell: (info) => (
          <span className="text-zinc-400">
            {formatRelativeTime(info.getValue())}
          </span>
        ),
        sortingFn: "datetime",
      }),
      col.display({
        id: "actions",
        header: "Actions",
        size: 160,
        cell: (info) => {
          const row = info.row.original;
          return (
            <div
              className="flex items-center gap-1"
              onClick={(e) => e.stopPropagation()}
            >
              {row.status === "complete" && (
                <>
                  <a
                    href={`#/runs/${row.id}`}
                    className="rounded px-2 py-1 text-xs text-emerald-400 transition-colors hover:bg-emerald-900/30 hover:text-emerald-300"
                    title="View results"
                  >
                    <ExternalLink className="inline h-3 w-3 mr-0.5" />
                    Results
                  </a>
                  <button
                    onClick={() => onRerun(row.id)}
                    disabled={rerunPending}
                    className="rounded px-2 py-1 text-xs text-blue-400 transition-colors hover:bg-blue-900/30 hover:text-blue-300 disabled:opacity-50"
                    title="Re-run this eval"
                  >
                    <RotateCcw className="inline h-3 w-3 mr-0.5" />
                    Re-Run
                  </button>
                </>
              )}
              {row.status === "failed" && (
                <button
                  onClick={() => onRerun(row.id)}
                  disabled={rerunPending}
                  className="rounded px-2 py-1 text-xs text-blue-400 transition-colors hover:bg-blue-900/30 hover:text-blue-300 disabled:opacity-50"
                  title="Re-run this eval"
                >
                  <RotateCcw className="inline h-3 w-3 mr-0.5" />
                  Re-Run
                </button>
              )}
              {(row.status === "queued" || row.status === "running") && (
                <button
                  onClick={() => onCancel(row.id)}
                  disabled={cancelPending}
                  className="rounded px-2 py-1 text-xs text-red-400 transition-colors hover:bg-red-900/30 hover:text-red-300 disabled:opacity-50"
                  title="Cancel run"
                >
                  <XCircle className="inline h-3 w-3 mr-0.5" />
                  Cancel
                </button>
              )}
            </div>
          );
        },
      }),
    ],
    [onCancel, onRerun, cancelPending, rerunPending],
  );

  const table = useReactTable({
    data,
    columns,
    state: { sorting },
    onSortingChange: setSorting,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
  });

  return (
    <div className="overflow-x-auto rounded-lg border border-zinc-700 bg-zinc-800">
      <table className="w-full text-left text-sm">
        <thead>
          {table.getHeaderGroups().map((hg) => (
            <tr key={hg.id} className="border-b border-zinc-700">
              {hg.headers.map((header) => (
                <th
                  key={header.id}
                  className="px-4 py-3 text-xs font-medium text-zinc-400 uppercase"
                  style={{
                    width:
                      header.getSize() !== 150 ? header.getSize() : undefined,
                  }}
                >
                  {header.isPlaceholder ? null : header.column.getCanSort() ? (
                    <button
                      className="flex items-center gap-1"
                      onClick={header.column.getToggleSortingHandler()}
                    >
                      {flexRender(
                        header.column.columnDef.header,
                        header.getContext(),
                      )}
                      <ArrowUpDown className="h-3 w-3" />
                    </button>
                  ) : (
                    flexRender(
                      header.column.columnDef.header,
                      header.getContext(),
                    )
                  )}
                </th>
              ))}
            </tr>
          ))}
        </thead>
        <tbody>
          {table.getRowModel().rows.map((row, i) => {
            const status = row.original.status;
            const isRunning = status === "running";
            const isActive = status === "queued" || isRunning;

            const navigateTo =
              status === "complete"
                ? `#/runs/${row.original.id}`
                : isActive
                  ? `#/runs/status/${row.original.id}`
                  : undefined;

            return (
              <tr
                key={row.id}
                className={`border-b border-zinc-700/50 ${
                  i % 2 === 0 ? "bg-zinc-800" : "bg-zinc-800/60"
                } ${navigateTo ? "cursor-pointer hover:bg-zinc-700/50" : ""} ${
                  isRunning ? "animate-pulse-subtle" : ""
                }`}
                onClick={() => {
                  if (navigateTo) window.location.hash = navigateTo;
                }}
                title={
                  row.original.error
                    ? `Error: ${row.original.error}`
                    : undefined
                }
              >
                {row.getVisibleCells().map((cell) => (
                  <td key={cell.id} className="px-4 py-3">
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </td>
                ))}
              </tr>
            );
          })}
        </tbody>
      </table>
      {data.length === 0 && (
        <div className="p-8 text-center text-zinc-500">
          No runs found.{" "}
          <a
            href="#/runs/new"
            className="text-blue-400 underline hover:text-blue-300"
          >
            Start a new eval run →
          </a>
        </div>
      )}
    </div>
  );
}

/* ── Dashboard (unified page) ─────────────────────────────────────── */

export default function Dashboard() {
  const summary = useSummary();
  const runs = useRuns();
  const results = useResults();
  const queue = useRunQueue();
  const cancelMutation = useCancelRun();
  const rerunMutation = useRerunRun();

  // Merge queue + results into a single unified list, dedup by ID
  // Queue items take precedence for in-progress runs
  const unifiedRuns = useMemo<UnifiedRun[]>(() => {
    const byId = new Map<string, UnifiedRun>();

    // First, map completed runs from both local and Cosmos sources
    const localRuns = runs.data ?? [];
    const cosmosResults = results.data ?? [];

    for (const r of localRuns) {
      byId.set(r.id, mapResultRun(r));
    }
    for (const r of cosmosResults) {
      const key = r.runId ?? r.id;
      if (!byId.has(key)) {
        byId.set(key, mapResultRun({ ...r, id: key }));
      }
    }

    // Then overlay queue items — queue takes precedence for active runs
    const queueItems = queue.data ?? [];
    for (const q of queueItems) {
      byId.set(q.id, mapQueueItem(q));
    }

    return Array.from(byId.values()).sort(
      (a, b) =>
        new Date(b.timestamp ?? 0).getTime() -
        new Date(a.timestamp ?? 0).getTime(),
    );
  }, [runs.data, results.data, queue.data]);

  const hasActiveRuns = unifiedRuns.some(
    (r) => r.status === "queued" || r.status === "running",
  );

  const isLoading = runs.isLoading && results.isLoading && queue.isLoading;
  const hasData = unifiedRuns.length > 0;

  // Build RunSummary[] for CSV export (only completed runs)
  const exportableRuns = useMemo<RunSummary[]>(() => {
    const localRuns = runs.data ?? [];
    const cosmosResults = results.data ?? [];
    const byId = new Map<string, RunSummary>();
    for (const run of localRuns) byId.set(run.id, run);
    for (const result of cosmosResults) {
      const key = result.runId ?? result.id;
      if (!byId.has(key)) {
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
    return Array.from(byId.values());
  }, [runs.data, results.data]);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-zinc-100">Eval Runs</h1>
        <div className="flex items-center gap-3">
          {hasActiveRuns && (
            <span className="flex items-center gap-1.5 text-xs text-zinc-500">
              Auto-refresh
              <span className="inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-blue-500" />
            </span>
          )}
          <button
            onClick={() => {
              void runs.refetch();
              void results.refetch();
              void queue.refetch();
            }}
            disabled={runs.isFetching || results.isFetching || queue.isFetching}
            className="flex items-center gap-1 rounded px-2 py-1 text-xs text-zinc-400 transition-colors hover:bg-zinc-700 hover:text-zinc-200 disabled:opacity-50"
          >
            <RefreshCw
              className={`h-3 w-3 ${
                runs.isFetching || results.isFetching || queue.isFetching
                  ? "animate-spin"
                  : ""
              }`}
            />
            Refresh
          </button>
          {hasData && (
            <button
              onClick={() => exportRunsToCSV(exportableRuns)}
              className="inline-flex items-center gap-1.5 rounded bg-zinc-700 px-3 py-1.5 text-sm text-zinc-100 hover:bg-zinc-600 transition-colors"
            >
              <Download className="h-3.5 w-3.5" />
              Export CSV
            </button>
          )}
        </div>
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
      {runs.isError && results.isError && queue.isError && (
        <ErrorBox
          message="Failed to load runs"
          onRetry={() => {
            void runs.refetch();
            void results.refetch();
            void queue.refetch();
          }}
        />
      )}
      {!isLoading && (
        <UnifiedRunsTable
          data={unifiedRuns}
          onCancel={(id) => cancelMutation.mutate(id)}
          onRerun={(id) => rerunMutation.mutate(id)}
          cancelPending={cancelMutation.isPending}
          rerunPending={rerunMutation.isPending}
        />
      )}
    </div>
  );
}
