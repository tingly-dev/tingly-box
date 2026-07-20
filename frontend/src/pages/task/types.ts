export interface TaskRecurrence { cron: string; timezone: string }

export type TaskAgent = 'claude' | 'codex' | 'shell';
export type TaskStatus = 'pending' | 'queued' | 'running' | 'needs_input' | 'handoff_required' | 'succeeded' | 'failed' | 'cancelled' | 'interrupted';
export type TaskRunStatus = 'running' | 'succeeded' | 'rescheduled' | 'needs_input' | 'handoff_required' | 'failed' | 'cancelled' | 'interrupted';
export type LaunchProfile = 'legacy_inherited' | 'plan' | 'manual' | 'accept_edits' | 'read_only' | 'workspace_write';
export type ToolCapability = 'files_read' | 'files_write' | 'terminal' | 'web';

export interface ExecutionPolicy {
  launch_profile: LaunchProfile;
  tools?: ToolCapability[];
}

export interface FollowUpPolicy {
  enabled: boolean;
  delay_seconds: number;
  max_wake_ups: number;
}

export interface TaskResult {
  state: 'done' | 'continue' | 'needs_input' | 'handoff_required';
  summary: string;
  question?: string;
  artifacts?: string[];
  native_session_id?: string;
  exit_code?: number;
  duration_ms?: number;
  exit_reason?: string;
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
  execution: ExecutionPolicy;
  active_run_id?: string;
}

export interface AgentAvailability {
  agent: TaskAgent;
  available: boolean;
  launch_profiles: LaunchProfile[];
  default_profile: LaunchProfile;
  tool_filtering: boolean;
  unattended: boolean;
}

export interface CreateTaskInput {
  title?: string;
  goal: string;
  agent: TaskAgent;
  workspace_path?: string;
  scheduled_at?: string;
  recurrence?: { cron: string; timezone: string };
  follow_up: FollowUpPolicy;
  timeout_seconds: number;
  steps?: Array<{ instruction: string }>;
  execution: ExecutionPolicy;
}

export interface UpdateTaskInput {
  title?: string;
  goal?: string;
  follow_up?: FollowUpPolicy;
  timeout_seconds?: number;
  execution?: ExecutionPolicy;
  steps?: { instruction: string }[];
  recurrence?: TaskRecurrence;
  clear_recurrence?: boolean;
}

export interface TaskRunEvent {
  id: string;
  kind: string;
  summary: string;
  data?: unknown;
  created_at: string;
}

export interface TaskRun {
  id: string;
  task_id: string;
  attempt: number;
  status: TaskRunStatus;
  trigger: 'run' | 'instruction' | 'step';
  step_id?: string;
  step_index?: number;
  instruction?: string;
  execution: ExecutionPolicy;
  progress?: string;
  result?: TaskResult;
  error?: string;
  events?: TaskRunEvent[];
  started_at: string;
  finished_at?: string;
  created_at: string;
  updated_at: string;
}

export interface TaskUsage {
  task_id: string;
  requests: number;
  input_tokens: number;
  output_tokens: number;
  cache_input_tokens: number;
  total_tokens: number;
}
