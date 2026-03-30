import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import {
  fetchSummary,
  fetchRuns,
  fetchRunDetail,
  fetchConnections,
  createConnection,
  testConnection,
  deleteConnection,
  fetchRepos,
  fetchRepoEvals,
  triggerRun,
  fetchRunQueue,
  cancelRun,
} from "../api/client";
import type { CreateConnectionRequest, TriggerRunConfig } from "../api/client";

export function useSummary() {
  return useQuery({
    queryKey: ["summary"],
    queryFn: fetchSummary,
  });
}

export function useRuns(sort = "timestamp", order = "desc") {
  return useQuery({
    queryKey: ["runs", sort, order],
    queryFn: () => fetchRuns(sort, order),
  });
}

export function useRunDetail(id: string) {
  return useQuery({
    queryKey: ["run", id],
    queryFn: () => fetchRunDetail(id),
    enabled: !!id,
  });
}

// --- Platform hooks ---

export function useConnections() {
  return useQuery({
    queryKey: ["connections"],
    queryFn: fetchConnections,
  });
}

export function useCreateConnection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: CreateConnectionRequest) => createConnection(data),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["connections"] });
    },
  });
}

export function useTestConnection() {
  return useMutation({
    mutationFn: (id: string) => testConnection(id),
  });
}

export function useDeleteConnection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteConnection(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["connections"] });
    },
  });
}

export function useRepos() {
  return useQuery({
    queryKey: ["repos"],
    queryFn: fetchRepos,
  });
}

export function useRepoEvals(owner: string, repo: string) {
  return useQuery({
    queryKey: ["repoEvals", owner, repo],
    queryFn: () => fetchRepoEvals(owner, repo),
    enabled: !!owner && !!repo,
  });
}

export function useTriggerRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (config: TriggerRunConfig) => triggerRun(config),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["runs"] });
      void qc.invalidateQueries({ queryKey: ["runQueue"] });
    },
  });
}

export function useRunQueue() {
  return useQuery({
    queryKey: ["runQueue"],
    queryFn: fetchRunQueue,
    refetchInterval: 5000,
  });
}

export function useCancelRun() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => cancelRun(id),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ["runQueue"] });
    },
  });
}
