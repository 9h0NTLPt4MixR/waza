import { ArrowLeft, Loader2, RefreshCw, XCircle } from "lucide-react";
import { useRunQueue, useCancelRun } from "../hooks/useApi";

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

function formatTime(iso: string | undefined | null): string {
  if (!iso) return "—";
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

export default function RunQueue() {
  const { data: runs, isLoading, isError, refetch, isFetching } = useRunQueue();
  const cancelMutation = useCancelRun();

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-4">
          <a
            href="#/"
            className="flex items-center gap-1 text-sm text-zinc-400 transition-colors hover:text-zinc-200"
          >
            <ArrowLeft className="h-4 w-4" />
            Dashboard
          </a>
          <span className="text-zinc-600">/</span>
          <h1 className="text-2xl font-semibold text-zinc-100">Run Queue</h1>
        </div>
        <div className="flex items-center gap-3">
          <span className="text-xs text-zinc-500">
            Auto-refresh 10s
            <span className="ml-1.5 inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-blue-500" />
          </span>
          <button
            onClick={() => refetch()}
            disabled={isFetching}
            className="flex items-center gap-1 rounded px-2 py-1 text-xs text-zinc-400 transition-colors hover:bg-zinc-700 hover:text-zinc-200 disabled:opacity-50"
          >
            <RefreshCw
              className={`h-3 w-3 ${isFetching ? "animate-spin" : ""}`}
            />
            Refresh
          </button>
        </div>
      </div>

      {/* Loading */}
      {isLoading && (
        <div className="flex items-center justify-center gap-3 py-16">
          <Loader2 className="h-6 w-6 animate-spin text-blue-400" />
          <span className="text-zinc-400">Loading queue…</span>
        </div>
      )}

      {/* Error */}
      {isError && (
        <div className="rounded-lg border border-red-800 bg-red-900/30 p-6 text-center">
          <XCircle className="mx-auto mb-2 h-8 w-8 text-red-400" />
          <p className="text-sm text-red-300">Failed to load run queue.</p>
        </div>
      )}

      {/* Empty */}
      {!isLoading && !isError && runs && runs.length === 0 && (
        <div className="rounded-lg border border-zinc-700 bg-zinc-800/30 p-8 text-center">
          <p className="text-sm text-zinc-400">No runs in the queue.</p>
          <a
            href="#/runs/new"
            className="mt-3 inline-block text-sm text-blue-400 underline hover:text-blue-300"
          >
            Start a new eval run →
          </a>
        </div>
      )}

      {/* Table */}
      {runs && runs.length > 0 && (
        <div className="overflow-x-auto rounded-lg border border-zinc-800">
          <table className="w-full text-left text-sm">
            <thead>
              <tr className="border-b border-zinc-800 bg-zinc-800/50">
                <th className="px-4 py-3 text-xs font-medium uppercase tracking-wider text-zinc-400">
                  Status
                </th>
                <th className="px-4 py-3 text-xs font-medium uppercase tracking-wider text-zinc-400">
                  Repo
                </th>
                <th className="px-4 py-3 text-xs font-medium uppercase tracking-wider text-zinc-400">
                  Eval
                </th>
                <th className="px-4 py-3 text-xs font-medium uppercase tracking-wider text-zinc-400">
                  Model
                </th>
                <th className="px-4 py-3 text-xs font-medium uppercase tracking-wider text-zinc-400">
                  Workers
                </th>
                <th className="px-4 py-3 text-xs font-medium uppercase tracking-wider text-zinc-400">
                  Storage
                </th>
                <th className="px-4 py-3 text-xs font-medium uppercase tracking-wider text-zinc-400">
                  Created
                </th>
                <th className="px-4 py-3 text-xs font-medium uppercase tracking-wider text-zinc-400">
                  Actions
                </th>
              </tr>
            </thead>
            <tbody>
              {runs.map((run) => (
                <tr
                  key={run.id}
                  onClick={() => {
                    window.location.hash = `/runs/status/${run.id}`;
                  }}
                  className="cursor-pointer border-b border-zinc-800/50 transition-colors hover:bg-zinc-800/40"
                >
                  <td className="px-4 py-3">
                    <StatusBadge status={run.status} />
                  </td>
                  <td className="px-4 py-3 font-mono text-zinc-200">
                    {run.repo ?? "—"}
                  </td>
                  <td className="px-4 py-3 font-mono text-zinc-200" title={run.evalSpec}>
                    {run.evalSpec
                      ? run.evalSpec.split("/").slice(-2).join("/")
                      : "—"}
                  </td>
                  <td className="px-4 py-3 font-mono text-zinc-300">
                    {run.model}
                  </td>
                  <td className="px-4 py-3 text-center text-zinc-300">
                    {run.workers}
                  </td>
                  <td className="px-4 py-3 text-zinc-400">
                    {run.storageDestination === "cosmos" ||
                    !run.storageDestination
                      ? "Waza Cloud"
                      : run.storageDestination}
                  </td>
                  <td className="px-4 py-3 text-zinc-400">
                    {formatTime(run.createdAt)}
                  </td>
                  <td className="px-4 py-3">
                    {(run.status === "queued" || run.status === "running") && (
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          cancelMutation.mutate(run.id);
                        }}
                        disabled={cancelMutation.isPending}
                        className="rounded px-2 py-1 text-xs text-red-400 transition-colors hover:bg-red-900/30 hover:text-red-300"
                        title="Cancel run"
                      >
                        Cancel
                      </button>
                    )}
                    {run.status === "complete" && (
                      <a
                        href={`#/runs/${run.id}`}
                        onClick={(e) => e.stopPropagation()}
                        className="rounded px-2 py-1 text-xs text-emerald-400 transition-colors hover:bg-emerald-900/30 hover:text-emerald-300"
                      >
                        Results
                      </a>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
