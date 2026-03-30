import { useState } from "react";
import {
  Plus,
  Trash2,
  CheckCircle,
  AlertTriangle,
  FlaskConical,
  Loader2,
} from "lucide-react";
import {
  useConnections,
  useCreateConnection,
  useTestConnection,
  useDeleteConnection,
} from "../hooks/useApi";
import type { Connection } from "../api/client";

type Tab = "connections" | "preferences";

function ConnectionBadge({ status }: { status: Connection["status"] }) {
  if (status === "verified") {
    return (
      <span className="inline-flex items-center gap-1 rounded-full bg-emerald-500/10 px-2 py-0.5 text-xs text-emerald-400">
        <CheckCircle className="h-3 w-3" /> Verified
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1 rounded-full bg-amber-500/10 px-2 py-0.5 text-xs text-amber-400">
      <AlertTriangle className="h-3 w-3" /> Unverified
    </span>
  );
}

function ConnectionRow({ conn }: { conn: Connection }) {
  const testMutation = useTestConnection();
  const deleteMutation = useDeleteConnection();

  return (
    <div className="flex items-center justify-between rounded-lg border border-zinc-800 bg-zinc-800/50 px-4 py-3">
      <div className="space-y-1">
        <div className="flex items-center gap-2">
          <span className="text-sm font-medium text-zinc-100">{conn.name}</span>
          <span className="rounded bg-zinc-700 px-1.5 py-0.5 text-xs text-zinc-400">
            {conn.type}
          </span>
          <ConnectionBadge status={conn.status} />
        </div>
        <p className="text-xs text-zinc-500">
          {conn.type === "azure-storage"
            ? `${conn.config.accountName}/${conn.config.containerName}`
            : `${conn.config.owner}/${conn.config.repo}`}
        </p>
      </div>
      <div className="flex items-center gap-2">
        <button
          onClick={() => testMutation.mutate(conn.id)}
          disabled={testMutation.isPending}
          className="inline-flex items-center gap-1 rounded bg-zinc-700 px-2.5 py-1.5 text-xs text-zinc-300 transition-colors hover:bg-zinc-600 disabled:opacity-50"
        >
          {testMutation.isPending ? (
            <Loader2 className="h-3 w-3 animate-spin" />
          ) : (
            <FlaskConical className="h-3 w-3" />
          )}
          Test
        </button>
        <button
          onClick={() => {
            if (confirm("Delete this connection?")) {
              deleteMutation.mutate(conn.id);
            }
          }}
          disabled={deleteMutation.isPending}
          className="inline-flex items-center gap-1 rounded bg-red-500/10 px-2.5 py-1.5 text-xs text-red-400 transition-colors hover:bg-red-500/20 disabled:opacity-50"
        >
          <Trash2 className="h-3 w-3" />
          Delete
        </button>
      </div>
    </div>
  );
}

function AddAzureStorageForm({ onDone }: { onDone: () => void }) {
  const [accountName, setAccountName] = useState("");
  const [containerName, setContainerName] = useState("");
  const createMutation = useCreateConnection();

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    createMutation.mutate(
      {
        type: "azure-storage",
        name: `${accountName}/${containerName}`,
        config: { accountName, containerName },
      },
      { onSuccess: onDone },
    );
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-3 rounded-lg border border-zinc-700 bg-zinc-800/30 p-4">
      <h4 className="text-sm font-medium text-zinc-200">Add Azure Storage</h4>
      <div className="grid grid-cols-2 gap-3">
        <input
          type="text"
          placeholder="Account name"
          value={accountName}
          onChange={(e) => setAccountName(e.target.value)}
          required
          className="rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-zinc-100 placeholder:text-zinc-500 focus:border-blue-500 focus:outline-none"
        />
        <input
          type="text"
          placeholder="Container name"
          value={containerName}
          onChange={(e) => setContainerName(e.target.value)}
          required
          className="rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-zinc-100 placeholder:text-zinc-500 focus:border-blue-500 focus:outline-none"
        />
      </div>
      <div className="flex gap-2">
        <button
          type="submit"
          disabled={createMutation.isPending}
          className="rounded bg-blue-600 px-3 py-1.5 text-sm text-white transition-colors hover:bg-blue-500 disabled:opacity-50"
        >
          {createMutation.isPending ? "Adding…" : "Add"}
        </button>
        <button
          type="button"
          onClick={onDone}
          className="rounded bg-zinc-700 px-3 py-1.5 text-sm text-zinc-300 transition-colors hover:bg-zinc-600"
        >
          Cancel
        </button>
      </div>
      {createMutation.isError && (
        <p className="text-xs text-red-400">
          {createMutation.error instanceof Error
            ? createMutation.error.message
            : "Failed to add connection"}
        </p>
      )}
    </form>
  );
}

function AddGitHubRepoForm({ onDone }: { onDone: () => void }) {
  const [ownerRepo, setOwnerRepo] = useState("");
  const createMutation = useCreateConnection();

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const [owner, repo] = ownerRepo.split("/");
    if (!owner || !repo) return;
    createMutation.mutate(
      {
        type: "github-repo",
        name: ownerRepo,
        config: { owner, repo },
      },
      { onSuccess: onDone },
    );
  };

  return (
    <form onSubmit={handleSubmit} className="space-y-3 rounded-lg border border-zinc-700 bg-zinc-800/30 p-4">
      <h4 className="text-sm font-medium text-zinc-200">Add GitHub Repo</h4>
      <input
        type="text"
        placeholder="owner/repo"
        value={ownerRepo}
        onChange={(e) => setOwnerRepo(e.target.value)}
        required
        pattern="[^/]+/[^/]+"
        title="Format: owner/repo"
        className="w-full rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-zinc-100 placeholder:text-zinc-500 focus:border-blue-500 focus:outline-none"
      />
      <div className="flex gap-2">
        <button
          type="submit"
          disabled={createMutation.isPending}
          className="rounded bg-blue-600 px-3 py-1.5 text-sm text-white transition-colors hover:bg-blue-500 disabled:opacity-50"
        >
          {createMutation.isPending ? "Adding…" : "Add"}
        </button>
        <button
          type="button"
          onClick={onDone}
          className="rounded bg-zinc-700 px-3 py-1.5 text-sm text-zinc-300 transition-colors hover:bg-zinc-600"
        >
          Cancel
        </button>
      </div>
      {createMutation.isError && (
        <p className="text-xs text-red-400">
          {createMutation.error instanceof Error
            ? createMutation.error.message
            : "Failed to add connection"}
        </p>
      )}
    </form>
  );
}

function ConnectionsTab() {
  const { data: connections, isLoading, isError } = useConnections();
  const [showForm, setShowForm] = useState<"azure" | "github" | null>(null);

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <h3 className="text-lg font-medium text-zinc-100">Connections</h3>
        <div className="flex gap-2">
          <button
            onClick={() => setShowForm("azure")}
            className="inline-flex items-center gap-1 rounded bg-zinc-700 px-3 py-1.5 text-sm text-zinc-100 transition-colors hover:bg-zinc-600"
          >
            <Plus className="h-3.5 w-3.5" />
            Azure Storage
          </button>
          <button
            onClick={() => setShowForm("github")}
            className="inline-flex items-center gap-1 rounded bg-zinc-700 px-3 py-1.5 text-sm text-zinc-100 transition-colors hover:bg-zinc-600"
          >
            <Plus className="h-3.5 w-3.5" />
            GitHub Repo
          </button>
        </div>
      </div>

      {showForm === "azure" && (
        <AddAzureStorageForm onDone={() => setShowForm(null)} />
      )}
      {showForm === "github" && (
        <AddGitHubRepoForm onDone={() => setShowForm(null)} />
      )}

      {isLoading && (
        <div className="space-y-3">
          {[1, 2].map((i) => (
            <div
              key={i}
              className="h-16 animate-pulse rounded-lg border border-zinc-800 bg-zinc-800/50"
            />
          ))}
        </div>
      )}

      {isError && (
        <div className="rounded-lg border border-red-500/30 bg-red-500/10 p-4 text-center">
          <p className="text-sm text-red-400">Failed to load connections</p>
        </div>
      )}

      {connections && connections.length === 0 && (
        <div className="rounded-lg border border-zinc-800 bg-zinc-800/30 p-8 text-center">
          <p className="text-sm text-zinc-400">
            No connections configured. Add an Azure Storage account or GitHub repo to get started.
          </p>
        </div>
      )}

      {connections && connections.length > 0 && (
        <div className="space-y-2">
          {connections.map((conn) => (
            <ConnectionRow key={conn.id} conn={conn} />
          ))}
        </div>
      )}
    </div>
  );
}

function PreferencesTab() {
  return (
    <div className="space-y-6">
      <h3 className="text-lg font-medium text-zinc-100">Preferences</h3>
      <div className="space-y-4">
        <div className="space-y-2">
          <label className="block text-sm font-medium text-zinc-300">
            Default Model
          </label>
          <select className="w-full max-w-xs rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-zinc-100 focus:border-blue-500 focus:outline-none">
            <option value="gpt-4o">gpt-4o</option>
            <option value="gpt-4o-mini">gpt-4o-mini</option>
            <option value="claude-sonnet-4">claude-sonnet-4</option>
            <option value="claude-opus-4">claude-opus-4</option>
          </select>
          <p className="text-xs text-zinc-500">
            Used as the default when creating new runs
          </p>
        </div>
        <div className="space-y-2">
          <label className="block text-sm font-medium text-zinc-300">
            Default Worker Count
          </label>
          <input
            type="number"
            min={1}
            max={10}
            defaultValue={3}
            className="w-full max-w-xs rounded border border-zinc-700 bg-zinc-800 px-3 py-2 text-sm text-zinc-100 focus:border-blue-500 focus:outline-none"
          />
          <p className="text-xs text-zinc-500">
            Number of parallel ADC sandboxes (1–10)
          </p>
        </div>
      </div>
      <p className="text-xs text-zinc-500 italic">
        Preferences are stored locally. Server-side persistence coming soon.
      </p>
    </div>
  );
}

export default function Settings() {
  const [tab, setTab] = useState<Tab>("connections");

  return (
    <div className="space-y-6">
      <h1 className="text-2xl font-semibold text-zinc-100">Settings</h1>
      <div className="flex gap-1 border-b border-zinc-800">
        {(["connections", "preferences"] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`border-b-2 px-4 py-2 text-sm capitalize transition-colors ${
              tab === t
                ? "border-blue-500 text-zinc-100"
                : "border-transparent text-zinc-400 hover:text-zinc-200"
            }`}
          >
            {t}
          </button>
        ))}
      </div>
      {tab === "connections" && <ConnectionsTab />}
      {tab === "preferences" && <PreferencesTab />}
    </div>
  );
}
