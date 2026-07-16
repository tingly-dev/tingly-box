// Temporary handwritten client for the experimental Task API. Replace these
// calls with the generated OpenAPI client after the backend schema is exported.
import { getApiBaseUrl } from '@/utils/protocol';
import { getControlApiHeaders } from '@/services/openapi';
import type { AgentAvailability, AgentTask, ControlDecision, CreateTaskInput, TaskRun } from '@/pages/task/types';

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const [baseUrl, authHeaders] = await Promise.all([getApiBaseUrl(), getControlApiHeaders()]);
  const response = await fetch(`${baseUrl}/api/v1${path}`, {
    ...init,
    headers: {
      'Content-Type': 'application/json',
      ...authHeaders,
      ...(init?.headers || {}),
    },
  });
  if (!response.ok) {
    const body = await response.json().catch(() => ({}));
    throw new Error(body.error || `Request failed (${response.status})`);
  }
  if (response.status === 204) return undefined as T;
  return response.json();
}

export const taskApi = {
  list: async (): Promise<AgentTask[]> => (await request<{ data: AgentTask[] }>('/tasks')).data,
  get: async (id: string): Promise<AgentTask> => (await request<{ data: AgentTask }>(`/tasks/${id}`)).data,
  agents: async (): Promise<AgentAvailability[]> => (await request<{ data: AgentAvailability[] }>('/tasks/agents')).data,
  create: async (input: CreateTaskInput): Promise<AgentTask> => (await request<{ data: AgentTask }>('/tasks', { method: 'POST', body: JSON.stringify(input) })).data,
  wake: async (id: string, instruction?: string): Promise<AgentTask> => (await request<{ data: AgentTask }>(`/tasks/${id}/wake`, { method: 'POST', body: JSON.stringify(instruction ? { instruction } : {}) })).data,
  stop: async (id: string): Promise<void> => request<void>(`/tasks/${id}/stop`, { method: 'POST' }),
  runs: async (id: string): Promise<TaskRun[]> => (await request<{ data: TaskRun[] }>(`/tasks/${id}/runs`)).data,
  respond: async (taskId: string, runId: string, controlId: string, decision: ControlDecision): Promise<TaskRun> => (
    await request<{ data: TaskRun }>(`/tasks/${taskId}/runs/${runId}/control/${controlId}/respond`, {
      method: 'POST', body: JSON.stringify(decision),
    })
  ).data,
};
