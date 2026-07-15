export type TaskAgent = 'claude' | 'codex';
export type TaskStatus = 'pending' | 'queued' | 'running' | 'needs_input' | 'succeeded' | 'failed' | 'cancelled' | 'interrupted';

export interface FollowUpPolicy {
  enabled: boolean;
  delay_seconds: number;
  max_wake_ups: number;
}

export interface TaskResult {
  state: 'done' | 'continue' | 'needs_input';
  summary: string;
  question?: string;
  artifacts?: string[];
  native_session_id?: string;
}

export interface TaskStep {
  id: string;
  title: string;
  instruction: string;
}

export interface StepOutcome {
  step_id: string;
  result: TaskResult;
  completed_at: string;
}

export interface AgentTask {
  id: string;
  title: string;
  goal: string;
  agent: TaskAgent;
  status: TaskStatus;
  progress?: string;
  error?: string;
  latest_result?: TaskResult;
  workspace_path: string;
  session_id?: string;
  resume_command?: string;
  follow_up: FollowUpPolicy;
  wake_count: number;
  scheduled_at?: string;
  started_at?: string;
  finished_at?: string;
  created_at: string;
  updated_at: string;
  recurrence?: { cron: string; timezone: string };
  steps?: TaskStep[];
  current_step: number;
  step_outcomes?: StepOutcome[];
}

export interface AgentAvailability {
  agent: TaskAgent;
  available: boolean;
}

export interface CreateTaskInput {
  title?: string;
  goal: string;
  agent: TaskAgent;
  scheduled_at?: string;
  recurrence?: { cron: string; timezone: string };
  follow_up: FollowUpPolicy;
  timeout_seconds: number;
  steps?: Array<{ instruction: string }>;
}
