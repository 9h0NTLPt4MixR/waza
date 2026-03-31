import { ArrowLeft, ExternalLink, Loader2, XCircle } from "lucide-react";
import { useRunStatus, useResultDetail } from "../hooks/useApi";
import { formatPercent } from "../lib/format";

const STATUS_CONFIG: Record<
  string,
  { emoji: string; label: string; color: string; bg: string }
> = {
  queued: {
    emoji: "🟡",
    label: "Queued",
    color: "text-yellow-300",
    bg: "bg-yellow-500/10 border-yellow-500/30",
  },
  running: {
    emoji: "🔵",
    label: "Running",
    color: "text-blue-300",
    bg: "bg-blue-500/10 border-blue-500/30",
  },
  complete: {
    emoji: "✅",
    label: "Complete",
    color: "text-emerald-300",
    bg: "bg-emerald-500/10 border-emerald-500/30",
  },
  failed: {
    emoji: "❌",
    label: "Failed",
    color: "text-red-300",
    bg: "bg-red-500/10 border-red-500/30",
  },
  cancelled: {
    emoji: "⚪",
    label: "Cancelled",
    color: "text-zinc-400",
    bg: "bg-zinc-500/10 border-zinc-500/30",
  },
};

function StatusBadge({ status }: { status: string }) {
  const cfg = STATUS_CONFIG[status] ?? STATUS_CONFIG.queued!;
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-sm font-medium ${cfg.bg} ${cfg.color}`}
    >
      {cfg.emoji} {cfg.label}
    </span>
  );
}

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleString();
  } catch {
    return iso;
  }
}

export default function RunStatus({ id }: { id: string }) {
  const { data: run, isLoading, isError } = useRunStatus(id);
  const isComplete = run?.status === "complete";
  const { data: resultDetail } = useResultDetail(isComplete ? id : "");

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-4">
        <a
          href="#/"
          className="flex items-center gap-1 text-sm text-zinc-400 transition-colors hover:text-zinc-200"
        >
          <ArrowLeft className="h-4 w-4" />
          Dashboard
        </a>
        <span className="text-zinc-600">/</span>
        <h1 className="text-2xl font-semibold text-zinc-100">Run Status</h1>
      </div>

      {/* Loading */}
      {isLoading && (
        <div className="flex items-center justify-center gap-3 py-16">
          <Loader2 className="h-6 w-6 animate-spin text-blue-400" />
          <span className="text-zinc-400">Loading run status…</span>
        </div>
      )}

      {/* Error */}
      {isError && (
        <div className="rounded-lg border border-red-800 bg-red-900/30 p-6 text-center">
          <XCircle className="mx-auto mb-2 h-8 w-8 text-red-400" />
          <p className="text-sm text-red-300">Failed to load run status.</p>
          <a
            href="#/"
            className="mt-3 inline-block text-sm text-blue-400 underline hover:text-blue-300"
          >
            Back to Dashboard
          </a>
        </div>
      )}

      {/* Not found */}
      {!isLoading && !isError && !run && (
        <div className="rounded-lg border border-zinc-700 bg-zinc-800/30 p-6 text-center">
          <p className="text-sm text-zinc-400">
            Run <span className="font-mono text-zinc-300">{id}</span> not found
            in queue.
          </p>
          <p className="mt-1 text-xs text-zinc-500">
            It may have been completed and moved to history.
          </p>
          <a
            href="#/"
            className="mt-3 inline-block text-sm text-blue-400 underline hover:text-blue-300"
          >
            Back to Dashboard
          </a>
        </div>
      )}

      {/* Run details */}
      {run && (
        <div className="space-y-6">
          {/* Status card */}
          <div className="rounded-lg border border-zinc-800 bg-zinc-800/30 p-6">
            <div className="mb-4 flex items-center justify-between">
              <StatusBadge status={run.status} />
              <span className="text-xs text-zinc-500">
                Polling every 3s
                <span className="ml-1.5 inline-block h-1.5 w-1.5 animate-pulse rounded-full bg-blue-500" />
              </span>
            </div>

            <div className="grid grid-cols-2 gap-x-8 gap-y-3 text-sm sm:grid-cols-3">
              <div>
                <span className="block text-xs font-medium uppercase tracking-wider text-zinc-500">
                  Run ID
                </span>
                <span className="font-mono text-zinc-200">{run.id}</span>
              </div>
              {run.repo && (
                <div>
                  <span className="block text-xs font-medium uppercase tracking-wider text-zinc-500">
                    Repository
                  </span>
                  <span className="font-mono text-zinc-200">{run.repo}</span>
                </div>
              )}
              <div>
                <span className="block text-xs font-medium uppercase tracking-wider text-zinc-500">
                  Eval Spec
                </span>
                <span className="font-mono text-zinc-200">{run.evalPath}</span>
              </div>
              <div>
                <span className="block text-xs font-medium uppercase tracking-wider text-zinc-500">
                  Model
                </span>
                <span className="font-mono text-zinc-200">{run.model}</span>
              </div>
              <div>
                <span className="block text-xs font-medium uppercase tracking-wider text-zinc-500">
                  Workers
                </span>
                <span className="font-mono text-zinc-200">{run.workers}</span>
              </div>
              <div>
                <span className="block text-xs font-medium uppercase tracking-wider text-zinc-500">
                  Storage
                </span>
                <span className="font-mono text-zinc-200">
                  {run.storageDestination === "cosmos" || !run.storageDestination
                    ? "Waza Cloud"
                    : run.storageDestination}
                </span>
              </div>
              <div>
                <span className="block text-xs font-medium uppercase tracking-wider text-zinc-500">
                  Created
                </span>
                <span className="text-zinc-200">
                  {formatTime(run.createdAt)}
                </span>
              </div>
            </div>
          </div>

          {/* Timeline / Log area */}
          <div className="rounded-lg border border-zinc-800 bg-zinc-800/30 p-6">
            <h2 className="mb-3 text-sm font-medium uppercase tracking-wider text-zinc-400">
              Execution Log
            </h2>
            <div className="min-h-[120px] rounded border border-zinc-700 bg-zinc-900/50 p-4 font-mono text-sm">
              {run.status === "queued" && (
                <div className="flex items-center gap-2 text-yellow-400/70">
                  <span className="inline-block h-2 w-2 animate-pulse rounded-full bg-yellow-400" />
                  Waiting for execution… Run is queued.
                </div>
              )}
              {run.status === "running" && (
                <div className="space-y-2">
                  <div className="flex items-center gap-2 text-blue-400/70">
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                    Evaluation in progress…
                  </div>
                  <div className="text-xs text-zinc-500">
                    Allocating {run.workers} ADC sandbox
                    {run.workers > 1 ? "es" : ""}…
                  </div>
                </div>
              )}
              {run.status === "complete" && (
                <div className="text-emerald-400/80">
                  ✅ Evaluation completed successfully.
                </div>
              )}
              {run.status === "failed" && (
                <div className="space-y-2">
                  <div className="text-red-400">❌ Evaluation failed.</div>
                  {run.error && (
                    <div className="rounded border border-red-800/50 bg-red-900/20 p-3 text-xs text-red-300">
                      {run.error}
                    </div>
                  )}
                </div>
              )}
              {run.status === "cancelled" && (
                <div className="text-zinc-500">
                  ⚪ Run was cancelled by user.
                </div>
              )}
            </div>
          </div>

          {/* Results summary (when complete + result data available) */}
          {isComplete && resultDetail && (
            <div className="rounded-lg border border-emerald-800/50 bg-emerald-500/5 p-6">
              <h2 className="mb-3 text-sm font-medium uppercase tracking-wider text-emerald-400">
                Results Summary
              </h2>
              <div className="grid grid-cols-2 gap-x-8 gap-y-3 text-sm sm:grid-cols-4">
                <div>
                  <span className="block text-xs font-medium uppercase tracking-wider text-zinc-500">
                    Pass Rate
                  </span>
                  <span className="text-lg font-semibold text-emerald-300">
                    {resultDetail.taskCount > 0
                      ? formatPercent(resultDetail.passCount / resultDetail.taskCount)
                      : "—"}
                  </span>
                </div>
                <div>
                  <span className="block text-xs font-medium uppercase tracking-wider text-zinc-500">
                    Tasks
                  </span>
                  <span className="text-lg font-semibold text-zinc-200">
                    {resultDetail.passCount}/{resultDetail.taskCount}
                  </span>
                </div>
                <div>
                  <span className="block text-xs font-medium uppercase tracking-wider text-zinc-500">
                    Tokens
                  </span>
                  <span className="text-lg font-semibold text-zinc-200">
                    {resultDetail.tokens.toLocaleString()}
                  </span>
                </div>
                <div>
                  <span className="block text-xs font-medium uppercase tracking-wider text-zinc-500">
                    Duration
                  </span>
                  <span className="text-lg font-semibold text-zinc-200">
                    {resultDetail.duration}s
                  </span>
                </div>
              </div>
            </div>
          )}

          {/* Action buttons */}
          <div className="flex items-center gap-3">
            {run.status === "complete" && (
              <a
                href={`#/runs/${run.id}`}
                className="inline-flex items-center gap-2 rounded bg-emerald-600 px-4 py-2 text-sm font-medium text-white transition-colors hover:bg-emerald-500"
              >
                <ExternalLink className="h-4 w-4" />
                View Results
              </a>
            )}
            <a
              href="#/runs/queue"
              className="inline-flex items-center gap-2 rounded bg-zinc-700 px-4 py-2 text-sm text-zinc-300 transition-colors hover:bg-zinc-600"
            >
              View Queue
            </a>
            <a
              href="#/"
              className="inline-flex items-center gap-2 rounded bg-zinc-700 px-4 py-2 text-sm text-zinc-300 transition-colors hover:bg-zinc-600"
            >
              <ArrowLeft className="h-4 w-4" />
              Dashboard
            </a>
          </div>
        </div>
      )}
    </div>
  );
}
