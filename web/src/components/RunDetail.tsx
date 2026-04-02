import { useState } from "react";
import {
  ArrowLeft,
  CheckCircle2,
  XCircle,
  AlertCircle,
  ChevronRight,
  ChevronDown,
  Download,
  RefreshCw,
  Loader2,
} from "lucide-react";
import { useRunDetail, useResultDetail, useRerunRun } from "../hooks/useApi";
import type { TaskResult, GraderResult } from "../api/client";
import {
  formatDuration,
  formatCost,
  formatNumber,
  formatPercent,
  formatRelativeTime,
} from "../lib/format";

/** Format a confidence interval as a percentage range string. */
function formatCIRange(lower: number, upper: number): string {
  const lo = (lower * 100).toFixed(1);
  const hi = (upper * 100).toFixed(1);
  return `[${lo}%, ${hi}%]`;
}
import { exportRunDetailToCSV } from "../lib/export";
import TrajectoryViewer from "./TrajectoryViewer";

/** Compute weighted score from grader results when not provided by backend. */
function computeWeightedScore(task: TaskResult): number | null {
  if (task.weightedScore != null) return task.weightedScore;
  const graders = task.graderResults;
  if (!graders || graders.length === 0) return null;
  const hasWeights = graders.some((g) => g.weight != null && g.weight !== 0);
  if (!hasWeights) return null;
  let totalWeight = 0;
  let weightedSum = 0;
  for (const g of graders) {
    const w = g.weight ?? 1;
    weightedSum += g.score * w;
    totalWeight += w;
  }
  return totalWeight > 0 ? weightedSum / totalWeight : null;
}

function OutcomeBadge({ outcome }: { outcome?: string }) {
  if (!outcome) return <span className="text-zinc-500">—</span>;
  if (outcome.startsWith("pass"))
    return (
      <span className="inline-flex items-center gap-1 rounded-full bg-green-500/10 px-2 py-0.5 text-xs font-medium text-green-500">
        <CheckCircle2 className="h-3 w-3" /> pass
      </span>
    );
  if (outcome.startsWith("fail"))
    return (
      <span className="inline-flex items-center gap-1 rounded-full bg-red-500/10 px-2 py-0.5 text-xs font-medium text-red-500">
        <XCircle className="h-3 w-3" /> fail
      </span>
    );
  return (
    <span className="inline-flex items-center gap-1 rounded-full bg-yellow-500/10 px-2 py-0.5 text-xs font-medium text-yellow-500">
      <AlertCircle className="h-3 w-3" /> error
    </span>
  );
}

function TypeBadge({ type }: { type: string }) {
  return (
    <span className="rounded bg-zinc-700 px-1.5 py-0.5 text-xs text-zinc-300">
      {type}
    </span>
  );
}

function SignificanceBadge({ isSignificant }: { isSignificant?: boolean }) {
  if (isSignificant == null) return null;
  if (isSignificant) {
    return (
      <span
        className="inline-flex items-center gap-0.5 rounded-full bg-green-500/10 px-1.5 py-0.5 text-xs font-medium text-green-400"
        data-testid="significance-badge"
        title="Statistically significant"
      >
        ✓ significant
      </span>
    );
  }
  return (
    <span
      className="inline-flex items-center gap-0.5 rounded-full bg-yellow-500/10 px-1.5 py-0.5 text-xs font-medium text-yellow-400"
      data-testid="significance-badge"
      title="Not statistically significant"
    >
      ⚠ not significant
    </span>
  );
}

function CIRange({ lower, upper }: { lower: number; upper: number }) {
  return (
    <span
      className="text-xs text-zinc-400"
      data-testid="ci-range"
      title={`95% CI: ${formatCIRange(lower, upper)}`}
    >
      {formatCIRange(lower, upper)}
    </span>
  );
}

function GraderRow({ grader }: { grader: GraderResult }) {
  const hasDetails = grader.details && Object.keys(grader.details).length > 0;
  return (
    <tr className="border-b border-zinc-700/30">
      <td className="py-2 pl-12 pr-4 text-zinc-300">{grader.name}</td>
      <td className="px-4 py-2">
        <TypeBadge type={grader.type} />
      </td>
      <td className="px-4 py-2">
        {grader.passed ? (
          <CheckCircle2 className="h-4 w-4 text-green-500" />
        ) : (
          <XCircle className="h-4 w-4 text-red-500" />
        )}
      </td>
      <td className="px-4 py-2 text-zinc-300">
        {formatPercent(grader.score)}
      </td>
      <td className="px-4 py-2 text-zinc-400">
        {grader.weight != null ? `×${grader.weight}` : "—"}
      </td>
      <td className="px-4 py-2 text-zinc-400">
        {grader.message && <span>{grader.message}</span>}
        {!grader.passed && hasDetails && (
          <pre className="mt-1 max-h-32 overflow-auto rounded bg-zinc-900 p-2 text-xs text-zinc-400">
            {Object.entries(grader.details!).map(([k, v]) => {
              const val = Array.isArray(v) ? v.join(", ") : String(v ?? "");
              return `${k}: ${val.slice(0, 200)}\n`;
            })}
          </pre>
        )}
      </td>
    </tr>
  );
}

function FailReason({ task }: { task: TaskResult }) {
  const allGradersPassed = task.graderResults.length > 0 && task.graderResults.every((g) => g.passed);
  if (!task.outcome?.startsWith("fail")) return null;
  // If some graders failed, the reason is obvious from the grader rows
  if (!allGradersPassed && task.graderResults.some((g) => !g.passed)) return null;

  const parts: string[] = [];
  if (task.passThreshold != null) {
    const score = task.weightedScore ?? task.score;
    parts.push(
      `Score ${formatPercent(score)} below threshold ${formatPercent(task.passThreshold)}`,
    );
  }
  if (task.numTrials != null && task.numTrials > 1) {
    const passed = task.passedTrials ?? 0;
    parts.push(`${passed} of ${task.numTrials} trials passed`);
  }
  if (parts.length === 0 && allGradersPassed) {
    // Fallback: all graders passed but task still failed
    const score = task.weightedScore ?? task.score;
    if (score < 1) {
      parts.push(`Weighted score ${formatPercent(score)} below pass threshold`);
    } else {
      parts.push("Trial-level failures reduced overall score");
    }
  }
  if (parts.length === 0) return null;
  return (
    <span className="ml-1.5 text-xs text-zinc-400">
      — {parts.join("; ")}
    </span>
  );
}

function TaskRow({ task }: { task: TaskResult }) {
  const [expanded, setExpanded] = useState(false);
  const ws = computeWeightedScore(task);

  return (
    <>
      <tr
        className="cursor-pointer border-b border-zinc-700/50 hover:bg-zinc-700/50"
        onClick={() => setExpanded(!expanded)}
      >
        <td className="px-4 py-3">
          <span className="flex items-center gap-2">
            {expanded ? (
              <ChevronDown className="h-4 w-4 text-zinc-500" />
            ) : (
              <ChevronRight className="h-4 w-4 text-zinc-500" />
            )}
            <span className="font-medium text-zinc-100">{task.name}</span>
          </span>
        </td>
        <td className="px-4 py-3">
          <span
            title={
              task.outcome?.startsWith("fail") && task.graderResults?.length > 0
                ? task.graderResults
                    .filter((g) => !g.passed)
                    .map((g) => `✗ ${g.name}${g.message ? ": " + g.message.slice(0, 120) : ""}`)
                    .join("\n") || "Failed"
                : undefined
            }
          >
            <OutcomeBadge outcome={task.outcome} />
            <FailReason task={task} />
          </span>
        </td>
        <td className="px-4 py-3 text-zinc-300">
          {formatPercent(task.score)}
        </td>
        <td className="px-4 py-3 text-zinc-300">
          <span className="flex items-center gap-1.5">
            {ws != null ? formatPercent(ws) : "—"}
            <SignificanceBadge isSignificant={task.isSignificant} />
          </span>
          {task.bootstrapCI && (
            <CIRange lower={task.bootstrapCI.lower} upper={task.bootstrapCI.upper} />
          )}
        </td>
        <td className="px-4 py-3 text-zinc-300">
          {formatDuration(task.duration)}
        </td>
      </tr>
      {expanded &&
        task.graderResults.map((g) => (
          <GraderRow key={g.name} grader={g} />
        ))}
    </>
  );
}

function DetailSkeleton() {
  return (
    <div className="space-y-6">
      <div className="h-5 w-24 rounded bg-zinc-700" />
      <div className="h-8 w-64 rounded bg-zinc-700" />
      <div className="flex gap-4">
        {Array.from({ length: 4 }).map((_, i) => (
          <div key={i} className="h-16 w-32 rounded-lg bg-zinc-800 border border-zinc-700" />
        ))}
      </div>
      <div className="h-48 rounded-lg bg-zinc-800 border border-zinc-700" />
    </div>
  );
}

export default function RunDetail({ id }: { id: string }) {
  const runQuery = useRunDetail(id);
  const resultQuery = useResultDetail(id);

  // Use local run data if available, fall back to Cosmos result
  const data = runQuery.data ?? resultQuery.data ?? null;
  const isLoading = runQuery.isLoading || (runQuery.isError && resultQuery.isLoading);
  const isError = runQuery.isError && resultQuery.isError;
  const error = runQuery.error ?? resultQuery.error;
  const refetch = () => { void runQuery.refetch(); void resultQuery.refetch(); };

  const [activeTab, setActiveTab] = useState<"tasks" | "trajectory">("tasks");
  const [trajectoryTask, setTrajectoryTask] = useState<TaskResult | null>(null);

  const rerunMutation = useRerunRun();
  const [rerunError, setRerunError] = useState<string | null>(null);

  const isRunComplete = data
    ? data.outcome?.startsWith("pass") ||
      data.outcome?.startsWith("fail") ||
      data.outcome?.startsWith("error")
    : false;

  function handleRerun() {
    if (!data) return;
    setRerunError(null);
    rerunMutation.mutate(data.id, {
      onSuccess: (result) => {
        window.location.hash = `/runs/status/${result.runId}`;
      },
      onError: (err) => {
        setRerunError(err instanceof Error ? err.message : "Rerun failed");
      },
    });
  }

  if (isLoading) return <DetailSkeleton />;

  if (isError) {
    return (
      <div className="space-y-4">
        <a href="#/" className="text-sm text-blue-500">
          <ArrowLeft className="mr-1 inline h-4 w-4" />
          Back to runs
        </a>
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-6 text-center">
          <p className="text-red-400">
            {error instanceof Error ? error.message : "Failed to load run"}
          </p>
          <button
            onClick={() => refetch()}
            className="mt-3 rounded bg-zinc-700 px-4 py-2 text-sm text-zinc-100"
          >
            Retry
          </button>
        </div>
      </div>
    );
  }

  if (!data) return null;

  const passRate =
    data.taskCount > 0 ? data.passCount / data.taskCount : 0;

  return (
    <div className="space-y-6">
      <a href="#/" className="inline-flex items-center gap-1 text-sm text-blue-500">
        <ArrowLeft className="h-4 w-4" />
        Back to runs
      </a>

      <div className="flex flex-wrap items-center gap-3">
        <h1 className="text-2xl font-semibold text-zinc-100">{data.spec}</h1>
        <OutcomeBadge outcome={data.outcome} />
        <span className="text-sm text-zinc-400">{data.model}</span>
        {data.judgeModel && (
          <span className="inline-flex items-center gap-1 rounded-full bg-purple-500/10 px-2 py-0.5 text-xs font-medium text-purple-400" data-testid="judge-model-badge">
            Judge: {data.judgeModel}
          </span>
        )}
        <span className="text-sm text-zinc-500">
          {formatRelativeTime(data.timestamp)}
        </span>
        <div className="ml-auto flex items-center gap-2">
          {isRunComplete && (
            <button
              onClick={handleRerun}
              disabled={rerunMutation.isPending}
              className="inline-flex items-center gap-1.5 rounded bg-blue-600 px-3 py-1.5 text-sm text-white hover:bg-blue-500 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {rerunMutation.isPending ? (
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              ) : (
                <RefreshCw className="h-3.5 w-3.5" />
              )}
              {rerunMutation.isPending ? "Re-Running…" : "Re-Run"}
            </button>
          )}
          <button
            onClick={() => exportRunDetailToCSV(data)}
            className="inline-flex items-center gap-1.5 rounded bg-zinc-700 px-3 py-1.5 text-sm text-zinc-100 hover:bg-zinc-600 transition-colors"
          >
            <Download className="h-3.5 w-3.5" />
            Export CSV
          </button>
        </div>
      </div>

      {rerunError && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 px-4 py-2 text-sm text-red-400">
          {rerunError}
        </div>
      )}

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <StatCard label="Pass Rate" value={formatPercent(passRate)} />
        <StatCard label="Tokens" value={formatNumber(data.tokens)} />
        <StatCard label="Cost" value={formatCost(data.cost)} />
        <StatCard label="Duration" value={formatDuration(data.duration)} />
      </div>

      <div className="flex gap-1 border-b border-zinc-700">
        <button
          onClick={() => { setActiveTab("tasks"); setTrajectoryTask(null); }}
          className={`px-4 py-2 text-sm font-medium transition-colors ${
            activeTab === "tasks"
              ? "border-b-2 border-blue-500 text-zinc-100"
              : "text-zinc-400 hover:text-zinc-200"
          }`}
        >
          Tasks
        </button>
        <button
          onClick={() => setActiveTab("trajectory")}
          className={`px-4 py-2 text-sm font-medium transition-colors ${
            activeTab === "trajectory"
              ? "border-b-2 border-blue-500 text-zinc-100"
              : "text-zinc-400 hover:text-zinc-200"
          }`}
        >
          Trajectory
        </button>
      </div>

      {activeTab === "tasks" && (
      <div className="overflow-x-auto rounded-lg border border-zinc-700 bg-zinc-800">
        <table className="w-full text-left text-sm">
          <thead>
            <tr className="border-b border-zinc-700">
              <th className="px-4 py-3 text-xs font-medium text-zinc-400 uppercase">
                Task
              </th>
              <th className="px-4 py-3 text-xs font-medium text-zinc-400 uppercase">
                Outcome
              </th>
              <th className="px-4 py-3 text-xs font-medium text-zinc-400 uppercase">
                Score
              </th>
              <th className="px-4 py-3 text-xs font-medium text-zinc-400 uppercase">
                W. Score
              </th>
              <th className="px-4 py-3 text-xs font-medium text-zinc-400 uppercase">
                Duration
              </th>
            </tr>
          </thead>
          <tbody>
            {data.tasks.map((task) => (
              <TaskRow key={task.name} task={task} />
            ))}
          </tbody>
        </table>
        {data.tasks.length === 0 && (
          <div className="p-8 text-center text-zinc-500">No tasks found.</div>
        )}
      </div>
      )}

      {activeTab === "trajectory" && (
        <div className="space-y-4">
          {!trajectoryTask ? (
            <div className="space-y-2">
              <p className="text-sm text-zinc-400">Select a task to view its trajectory:</p>
              {data.tasks.map((task) => (
                <button
                  key={task.name}
                  onClick={() => setTrajectoryTask(task)}
                  className="flex w-full items-center justify-between rounded-lg border border-zinc-700 bg-zinc-800 px-4 py-3 text-left hover:bg-zinc-700/50 transition-colors"
                >
                  <span className="font-medium text-zinc-100">{task.name}</span>
                  <OutcomeBadge outcome={task.outcome} />
                </button>
              ))}
              {data.tasks.length === 0 && (
                <p className="text-sm text-zinc-500">No tasks available.</p>
              )}
            </div>
          ) : (
            <div className="space-y-3">
              <button
                onClick={() => setTrajectoryTask(null)}
                className="inline-flex items-center gap-1 text-sm text-blue-500 hover:text-blue-400"
              >
                <ArrowLeft className="h-3.5 w-3.5" />
                Back to task list
              </button>
              <TrajectoryViewer task={trajectoryTask} />
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function StatCard({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-lg border border-zinc-700 bg-zinc-800 p-3">
      <p className="text-xs text-zinc-400">{label}</p>
      <p className="mt-1 text-lg font-semibold text-zinc-100">{value}</p>
    </div>
  );
}
