import { useState } from "react";
import { Play, ChevronRight, Loader2, RefreshCw, Database } from "lucide-react";
import { useRepos, useRepoEvals, useTriggerRun, useConnections } from "../hooks/useApi";

type Step = 1 | 2 | 3 | 4;

const MODELS = [
  "gpt-4o",
  "gpt-4o-mini",
  "claude-sonnet-4",
  "claude-opus-4",
  "o3-mini",
];

function StepIndicator({ current, total }: { current: number; total: number }) {
  return (
    <div className="flex items-center gap-2">
      {Array.from({ length: total }, (_, i) => {
        const step = i + 1;
        const active = step === current;
        const done = step < current;
        return (
          <div key={step} className="flex items-center gap-2">
            <div
              className={`flex h-7 w-7 items-center justify-center rounded-full text-xs font-medium ${
                active
                  ? "bg-blue-600 text-white"
                  : done
                    ? "bg-emerald-600 text-white"
                    : "bg-zinc-700 text-zinc-400"
              }`}
            >
              {done ? "✓" : step}
            </div>
            {step < total && (
              <ChevronRight className="h-4 w-4 text-zinc-600" />
            )}
          </div>
        );
      })}
    </div>
  );
}

export default function NewRun() {
  const [step, setStep] = useState<Step>(1);
  const [selectedRepo, setSelectedRepo] = useState("");
  const [selectedEval, setSelectedEval] = useState("");
  const [model, setModel] = useState("gpt-4o");
  const [workers, setWorkers] = useState(3);
  const [parallel, setParallel] = useState(true);
  const [storageDestination, setStorageDestination] = useState("cosmos");

  const repos = useRepos();
  const connections = useConnections();
  const [owner, repo] = selectedRepo.split("/");
  const evals = useRepoEvals(owner ?? "", repo ?? "");
  const triggerMutation = useTriggerRun();

  const handleTrigger = () => {
    if (!owner || !repo || !selectedEval) return;
    triggerMutation.mutate(
      {
        owner,
        repo,
        evalPath: selectedEval,
        model,
        workers,
        parallel,
        storageDestination,
      },
      {
        onSuccess: (data) => {
          window.location.hash = `/runs/status/${data.runId}`;
        },
      },
    );
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-semibold text-zinc-100">New Run</h1>
        <StepIndicator current={step} total={4} />
      </div>

      <div className="rounded-lg border border-zinc-800 bg-zinc-800/30 p-6">
        {/* Step 1: Select Source */}
        {step === 1 && (
          <div className="space-y-4">
            <div className="flex items-center justify-between">
              <h2 className="text-lg font-medium text-zinc-100">
                Select Source Repository
              </h2>
              <button
                onClick={() => repos.refetch()}
                disabled={repos.isFetching}
                className="flex items-center gap-1 rounded px-2 py-1 text-xs text-zinc-400 hover:text-zinc-200 hover:bg-zinc-700 transition-colors disabled:opacity-50"
                title="Refresh repos"
              >
                <RefreshCw className={`h-3 w-3 ${repos.isFetching ? "animate-spin" : ""}`} />
                Refresh
              </button>
            </div>
            <p className="text-sm text-zinc-400">
              Choose from your connected GitHub repos
            </p>
            {repos.isLoading && (
              <div className="flex items-center gap-2 text-sm text-zinc-400">
                <Loader2 className="h-4 w-4 animate-spin" />
                Loading repos…
              </div>
            )}
            {repos.isError && (
              <div className="rounded border border-red-800 bg-red-900/30 p-4 text-sm text-red-300">
                <p>Failed to load repos.</p>
                <a
                  href="#/settings"
                  className="mt-2 inline-block text-blue-400 hover:text-blue-300 underline"
                >
                  → Check your connections in Settings
                </a>
              </div>
            )}
            {repos.data && repos.data.length === 0 && (
              <div className="rounded border border-zinc-700 bg-zinc-800/50 p-4 text-sm text-zinc-400">
                <p>No repositories connected yet.</p>
                <a
                  href="#/settings"
                  className="mt-2 inline-block text-blue-400 hover:text-blue-300 underline"
                >
                  → Add a GitHub repo in Settings
                </a>
              </div>
            )}
            {repos.data && repos.data.length > 0 && (
              <select
                value={selectedRepo}
                onChange={(e) => {
                  setSelectedRepo(e.target.value);
                  setSelectedEval("");
                }}
                className="w-full max-w-md rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-zinc-100 focus:border-blue-500 focus:outline-none"
              >
                <option value="">Select a repository…</option>
                {repos.data.map((r) => (
                  <option key={r.fullName} value={r.fullName}>
                    {r.fullName}
                  </option>
                ))}
              </select>
            )}
            <div className="flex justify-end">
              <button
                onClick={() => setStep(2)}
                disabled={!selectedRepo}
                className="rounded bg-blue-600 px-4 py-2 text-sm text-white transition-colors hover:bg-blue-500 disabled:opacity-50"
              >
                Next
              </button>
            </div>
          </div>
        )}

        {/* Step 2: Select Eval Spec */}
        {step === 2 && (
          <div className="space-y-4">
            <h2 className="text-lg font-medium text-zinc-100">
              Select Eval Spec
            </h2>
            <p className="text-sm text-zinc-400">
              Choose the evaluation to run from{" "}
              <span className="font-mono text-zinc-300">{selectedRepo}</span>
            </p>
            {evals.isLoading && (
              <div className="flex items-center gap-2 text-sm text-zinc-400">
                <Loader2 className="h-4 w-4 animate-spin" />
                Discovering evals…
              </div>
            )}
            {evals.data && (
              <select
                value={selectedEval}
                onChange={(e) => setSelectedEval(e.target.value)}
                className="w-full max-w-md rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-zinc-100 focus:border-blue-500 focus:outline-none"
              >
                <option value="">Select an eval…</option>
                {evals.data.map((ev) => (
                  <option key={ev.path} value={ev.path}>
                    {ev.name} — {ev.path}
                  </option>
                ))}
              </select>
            )}
            <div className="flex justify-between">
              <button
                onClick={() => setStep(1)}
                className="rounded bg-zinc-700 px-4 py-2 text-sm text-zinc-300 transition-colors hover:bg-zinc-600"
              >
                Back
              </button>
              <button
                onClick={() => setStep(3)}
                disabled={!selectedEval}
                className="rounded bg-blue-600 px-4 py-2 text-sm text-white transition-colors hover:bg-blue-500 disabled:opacity-50"
              >
                Next
              </button>
            </div>
          </div>
        )}

        {/* Step 3: Configure */}
        {step === 3 && (
          <div className="space-y-4">
            <h2 className="text-lg font-medium text-zinc-100">
              Configure Run
            </h2>
            <div className="grid max-w-md gap-4">
              <div className="space-y-1">
                <label className="block text-sm font-medium text-zinc-300">
                  Model
                </label>
                <select
                  value={model}
                  onChange={(e) => setModel(e.target.value)}
                  className="w-full rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-zinc-100 focus:border-blue-500 focus:outline-none"
                >
                  {MODELS.map((m) => (
                    <option key={m} value={m}>
                      {m}
                    </option>
                  ))}
                </select>
              </div>
              <div className="space-y-1">
                <label className="block text-sm font-medium text-zinc-300">
                  Workers (ADC sandboxes)
                </label>
                <input
                  type="number"
                  min={1}
                  max={10}
                  value={workers}
                  onChange={(e) =>
                    setWorkers(
                      Math.min(10, Math.max(1, Number(e.target.value))),
                    )
                  }
                  className="w-full rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-zinc-100 focus:border-blue-500 focus:outline-none"
                />
                <p className="text-xs text-zinc-500">
                  Max 10 sandboxes per quota
                </p>
              </div>
              <label className="flex items-center gap-2">
                <input
                  type="checkbox"
                  checked={parallel}
                  onChange={(e) => setParallel(e.target.checked)}
                  className="h-4 w-4 rounded border-zinc-600 bg-zinc-800 text-blue-600 focus:ring-blue-500"
                />
                <span className="text-sm text-zinc-300">
                  Run tasks in parallel
                </span>
              </label>

              {/* Results Storage */}
              <div className="space-y-1">
                <label className="block text-sm font-medium text-zinc-300">
                  Results Storage
                </label>
                {(() => {
                  const storageConnections = (connections.data ?? []).filter(
                    (c) => c.type === "azure-storage",
                  );
                  if (storageConnections.length === 0) {
                    return (
                      <div className="flex items-center gap-2 rounded border border-zinc-700 bg-zinc-800/50 px-3 py-2 text-sm text-zinc-300">
                        <Database className="h-4 w-4 text-emerald-400" />
                        💾 Results stored in Waza Cloud
                      </div>
                    );
                  }
                  return (
                    <select
                      value={storageDestination}
                      onChange={(e) => setStorageDestination(e.target.value)}
                      className="w-full rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-zinc-100 focus:border-blue-500 focus:outline-none"
                    >
                      <option value="cosmos">Waza Cloud (default)</option>
                      {storageConnections.map((c) => {
                        const account = c.config["account_name"] ?? "storage";
                        const container = c.config["container_name"] ?? "results";
                        return (
                          <option key={c.id} value={c.id}>
                            {account}/{container}
                          </option>
                        );
                      })}
                    </select>
                  );
                })()}
                <p className="text-xs text-zinc-500">
                  Cosmos DB is always available. Connect Azure Storage in Settings for BYOS.
                </p>
              </div>
            </div>
            <div className="rounded border border-zinc-700 bg-zinc-800/50 p-3">
              <p className="text-xs text-zinc-400">
                Estimated sandboxes: <span className="font-mono text-zinc-200">{workers}</span>
              </p>
            </div>
            <div className="flex justify-between">
              <button
                onClick={() => setStep(2)}
                className="rounded bg-zinc-700 px-4 py-2 text-sm text-zinc-300 transition-colors hover:bg-zinc-600"
              >
                Back
              </button>
              <button
                onClick={() => setStep(4)}
                className="rounded bg-blue-600 px-4 py-2 text-sm text-white transition-colors hover:bg-blue-500"
              >
                Review
              </button>
            </div>
          </div>
        )}

        {/* Step 4: Review & Run */}
        {step === 4 && (
          <div className="space-y-4">
            <h2 className="text-lg font-medium text-zinc-100">
              Review &amp; Run
            </h2>
            <div className="space-y-2 rounded border border-zinc-700 bg-zinc-800/50 p-4">
              <div className="grid grid-cols-2 gap-y-2 text-sm">
                <span className="text-zinc-400">Repository</span>
                <span className="font-mono text-zinc-100">{selectedRepo}</span>
                <span className="text-zinc-400">Eval</span>
                <span className="font-mono text-zinc-100">{selectedEval}</span>
                <span className="text-zinc-400">Model</span>
                <span className="font-mono text-zinc-100">{model}</span>
                <span className="text-zinc-400">Workers</span>
                <span className="font-mono text-zinc-100">{workers}</span>
                <span className="text-zinc-400">Parallel</span>
                <span className="font-mono text-zinc-100">
                  {parallel ? "Yes" : "No"}
                </span>
                <span className="text-zinc-400">Storage</span>
                <span className="font-mono text-zinc-100">
                  {storageDestination === "cosmos"
                    ? "Waza Cloud (Cosmos DB)"
                    : (() => {
                        const conn = (connections.data ?? []).find(
                          (c) => c.id === storageDestination,
                        );
                        if (!conn) return storageDestination;
                        const account = conn.config["account_name"] ?? "storage";
                        const container = conn.config["container_name"] ?? "results";
                        return `${account}/${container}`;
                      })()}
                </span>
              </div>
            </div>
            {triggerMutation.isError && (
              <div className="rounded border border-red-500/30 bg-red-500/10 p-3">
                <p className="text-sm text-red-400">
                  {triggerMutation.error instanceof Error
                    ? triggerMutation.error.message
                    : "Failed to trigger run"}
                </p>
              </div>
            )}
            <div className="flex justify-between">
              <button
                onClick={() => setStep(3)}
                className="rounded bg-zinc-700 px-4 py-2 text-sm text-zinc-300 transition-colors hover:bg-zinc-600"
              >
                Back
              </button>
              <button
                onClick={handleTrigger}
                disabled={triggerMutation.isPending}
                className="inline-flex items-center gap-2 rounded bg-emerald-600 px-5 py-2 text-sm font-medium text-white transition-colors hover:bg-emerald-500 disabled:opacity-50"
              >
                {triggerMutation.isPending ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <Play className="h-4 w-4" />
                )}
                Run Evaluation
              </button>
            </div>
          </div>
        )}
      </div>
    </div>
  );
}
