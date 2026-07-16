import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert, Box, Button, Checkbox, Chip, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle,
  Divider, FormControlLabel, FormGroup, IconButton, Paper, Stack, Switch,
  TextField, ToggleButton, ToggleButtonGroup, Tooltip, Typography,
} from '@mui/material';
import PageHeader from '@/components/PageHeader';
import { Add, Block, Check, ContentCopy, Delete, History, PlayArrow, Refresh, Schedule, Security, Send, Terminal } from '@/components/icons';
import { useFeatureFlags } from '@/contexts/FeatureFlagsContext';
import { taskApi } from '@/services/taskApi';
import type {
  AgentAvailability, AgentTask, ControlDecision, CreateTaskInput, ExecutionPolicy, LaunchProfile,
  PendingControl, TaskAgent, TaskRun, TaskRunStatus, TaskStatus, ToolCapability,
} from './types';

const statusMeta: Record<TaskStatus, { label: string; color: 'default' | 'primary' | 'warning' | 'success' | 'error' }> = {
  pending: { label: 'Waiting', color: 'default' }, queued: { label: 'Queued', color: 'default' },
  running: { label: 'Working', color: 'primary' }, needs_input: { label: 'Needs you', color: 'warning' },
  succeeded: { label: 'Done', color: 'success' }, failed: { label: 'Failed', color: 'error' },
  cancelled: { label: 'Stopped', color: 'default' }, interrupted: { label: 'Interrupted', color: 'error' },
};

const runStatusMeta: Record<TaskRunStatus, { label: string; color: 'default' | 'primary' | 'warning' | 'success' | 'error' }> = {
  running: { label: 'Working', color: 'primary' }, waiting_approval: { label: 'Approval needed', color: 'warning' },
  waiting_input: { label: 'Answer needed', color: 'warning' }, succeeded: { label: 'Done', color: 'success' },
  rescheduled: { label: 'Continues later', color: 'primary' }, needs_input: { label: 'Paused', color: 'warning' },
  failed: { label: 'Failed', color: 'error' }, cancelled: { label: 'Stopped', color: 'default' },
  interrupted: { label: 'Interrupted', color: 'error' },
};

const profileMeta: Record<LaunchProfile, { label: string; description: string }> = {
  legacy_inherited: { label: 'Inherited', description: 'Use the CLI configuration already installed on this machine.' },
  plan: { label: 'Review only', description: 'Claude can inspect and propose a plan, but cannot change the workspace.' },
  manual: { label: 'Ask before actions', description: 'Claude pauses for approval when an action requires permission.' },
  accept_edits: { label: 'Edit workspace', description: 'Workspace edits are allowed; other protected actions still ask.' },
  read_only: { label: 'Review only', description: 'Codex runs in its read-only sandbox.' },
  workspace_write: { label: 'Edit workspace', description: 'Codex can write inside this task workspace.' },
};

const getProfileMeta = (profile: string) => profileMeta[profile as LaunchProfile] || {
  label: profile.replaceAll('_', ' '),
  description: `The CLI will start with the ${profile} profile.`,
};

const toolMeta: Record<ToolCapability, string> = {
  files_read: 'Read files', files_write: 'Edit files', terminal: 'Run commands', web: 'Use the web',
};

const defaultExecution = (agent: TaskAgent): ExecutionPolicy => agent === 'claude'
  ? { launch_profile: 'accept_edits', tools: ['files_read', 'files_write', 'terminal'] }
  : { launch_profile: 'workspace_write' };

const formatTime = (value?: string) => value ? new Intl.DateTimeFormat(undefined, {
  month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
}).format(new Date(value)) : '—';

const isActive = (task: AgentTask) => ['pending', 'queued', 'running'].includes(task.status);
const canStop = (task: AgentTask) => ['pending', 'queued', 'running', 'needs_input'].includes(task.status);

function CreateTaskDialog({ open, agents, onClose, onCreated }: {
  open: boolean; agents: AgentAvailability[]; onClose: () => void; onCreated: (task: AgentTask) => void;
}) {
  const [goal, setGoal] = useState('');
  const [agent, setAgent] = useState<TaskAgent>('claude');
  const [execution, setExecution] = useState<ExecutionPolicy>(defaultExecution('claude'));
  const [when, setWhen] = useState<'now' | 'later' | 'repeat'>('now');
  const [scheduledAt, setScheduledAt] = useState('');
  const [cron, setCron] = useState('0 9 * * *');
  const [keepChecking, setKeepChecking] = useState(false);
  const [delay, setDelay] = useState(300);
  const [maxWakeUps, setMaxWakeUps] = useState(20);
  const [steps, setSteps] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const agentInfo = (kind: TaskAgent) => agents.find((item) => item.agent === kind);
  const availability = (kind: TaskAgent) => agentInfo(kind)?.available ?? false;

  const chooseAgent = (kind: TaskAgent) => {
    setAgent(kind);
    const info = agentInfo(kind);
    const defaults = defaultExecution(kind);
    setExecution({ ...defaults, launch_profile: info?.default_profile || defaults.launch_profile });
  };

  useEffect(() => {
    if (availability(agent)) return;
    const firstAvailable = agents.find((item) => item.available);
    if (firstAvailable) chooseAgent(firstAvailable.agent);
  }, [agent, agents]);

  const chooseProfile = (profile: LaunchProfile) => {
    setExecution((current) => {
      if (profile === 'plan') {
        return { launch_profile: profile, tools: (current.tools || []).filter((tool) => tool !== 'files_write') };
      }
      return { ...current, launch_profile: profile };
    });
  };

  const toggleTool = (tool: ToolCapability) => setExecution((current) => {
    const selected = current.tools || [];
    return { ...current, tools: selected.includes(tool) ? selected.filter((item) => item !== tool) : [...selected, tool] };
  });

  const submit = async () => {
    if (!goal.trim()) return;
    setSaving(true); setError('');
    const input: CreateTaskInput = {
      goal: goal.trim(), agent,
      follow_up: { enabled: keepChecking, delay_seconds: delay, max_wake_ups: maxWakeUps },
      timeout_seconds: 1800,
      execution,
    };
    if (steps.length) input.steps = steps.map((instruction) => ({ instruction: instruction.trim() }));
    if (when === 'later' && scheduledAt) input.scheduled_at = new Date(scheduledAt).toISOString();
    if (when === 'repeat') input.recurrence = { cron: cron.trim(), timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC' };
    try {
      const created = await taskApi.create(input);
      setGoal(''); setSteps([]); onCreated(created); onClose();
    } catch (err) { setError((err as Error).message); } finally { setSaving(false); }
  };

  return <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
    <DialogTitle>New task</DialogTitle>
    <DialogContent sx={{ pt: '12px !important' }}>
      <Stack spacing={2.5}>
        {error && <Alert severity="error">{error}</Alert>}
        <TextField autoFocus multiline minRows={3} label="What should be done?" value={goal} onChange={(e) => setGoal(e.target.value)} fullWidth />
        <Box>
          <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: steps.length ? 1.5 : 0 }}>
            <Box><Typography variant="subtitle2">Steps <Typography component="span" variant="body2" color="text.secondary">(optional)</Typography></Typography>{!steps.length && <Typography variant="caption" color="text.secondary">Run explicit steps separately, in order.</Typography>}</Box>
            <Button size="small" startIcon={<Add />} disabled={steps.length >= 50} onClick={() => setSteps((items) => [...items, ''])}>Add step</Button>
          </Stack>
          {steps.length > 0 && <Stack spacing={1.25}>
            <Typography variant="caption" color="text.secondary">Each step gets its own run and reuses this task's workspace and session.</Typography>
            {steps.map((step, index) => <Stack key={index} direction="row" spacing={1} alignItems="flex-start">
              <TextField multiline minRows={2} fullWidth label={`Step ${index + 1}`} value={step} onChange={(e) => setSteps((items) => items.map((item, itemIndex) => itemIndex === index ? e.target.value : item))} />
              <Tooltip title="Remove step"><IconButton aria-label={`Remove step ${index + 1}`} onClick={() => setSteps((items) => items.filter((_, itemIndex) => itemIndex !== index))} sx={{ mt: 1 }}><Delete fontSize="small" /></IconButton></Tooltip>
            </Stack>)}
          </Stack>}
        </Box>
        <Box>
          <Typography variant="subtitle2" sx={{ mb: 1 }}>Agent</Typography>
          <ToggleButtonGroup exclusive value={agent} onChange={(_, value) => value && chooseAgent(value)} fullWidth>
            {(['claude', 'codex'] as TaskAgent[]).map((kind) => <ToggleButton key={kind} value={kind} disabled={!availability(kind)} sx={{ textTransform: 'none' }}>
              <Stack direction="row" spacing={1} alignItems="center"><Terminal fontSize="small" /><span>{kind === 'claude' ? 'Claude Code' : 'Codex'}</span><Chip size="small" label={availability(kind) ? 'Available' : 'Not found'} color={availability(kind) ? 'success' : 'default'} variant="outlined" /></Stack>
            </ToggleButton>)}
          </ToggleButtonGroup>
        </Box>
        <Box>
          <Typography variant="subtitle2">How it can work</Typography>
          <Typography variant="caption" color="text.secondary">This applies when the CLI starts. Individual approvals appear on the task while it is running.</Typography>
          <ToggleButtonGroup exclusive value={execution.launch_profile} onChange={(_, value) => value && chooseProfile(value)} fullWidth size="small" sx={{ mt: 1.25 }}>
            {(agentInfo(agent)?.launch_profiles || (agent === 'claude'
              ? ['plan', 'manual', 'accept_edits']
              : ['read_only', 'workspace_write'])).map((profile) => <ToggleButton key={profile} value={profile} sx={{ textTransform: 'none' }}>
                {getProfileMeta(profile).label}
              </ToggleButton>)}
          </ToggleButtonGroup>
          <Alert icon={<Security fontSize="inherit" />} severity="info" sx={{ mt: 1.25 }}>
            {getProfileMeta(execution.launch_profile).description}
          </Alert>
        </Box>
        {agent === 'claude' ? <Box>
          <Typography variant="subtitle2">Tools Claude may use</Typography>
          <FormGroup row sx={{ mt: 0.5 }}>
            {(Object.keys(toolMeta) as ToolCapability[]).map((tool) => <FormControlLabel key={tool} label={toolMeta[tool]} control={<Checkbox
              size="small" checked={execution.tools?.includes(tool) || false}
              disabled={tool === 'files_write' && execution.launch_profile === 'plan'}
              onChange={() => toggleTool(tool)}
            />} />)}
          </FormGroup>
        </Box> : <Alert severity="info">Codex uses its sandbox for file access. Per-task tool filtering is not exposed by the Codex CLI integration.</Alert>}
        <Box>
          <Typography variant="subtitle2" sx={{ mb: 1 }}>When</Typography>
          <ToggleButtonGroup exclusive value={when} onChange={(_, value) => value && setWhen(value)} fullWidth size="small">
            <ToggleButton value="now">Now</ToggleButton><ToggleButton value="later">Later</ToggleButton><ToggleButton value="repeat">Repeat</ToggleButton>
          </ToggleButtonGroup>
        </Box>
        {when === 'later' && <TextField type="datetime-local" label="Start at" value={scheduledAt} onChange={(e) => setScheduledAt(e.target.value)} slotProps={{ inputLabel: { shrink: true } }} />}
        {when === 'repeat' && <Stack spacing={1}>
          <TextField label="Cron schedule" value={cron} onChange={(e) => setCron(e.target.value)} helperText={`Five fields · ${Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'}`} />
        </Stack>}
        <Divider />
        <FormControlLabel control={<Switch checked={keepChecking} onChange={(e) => setKeepChecking(e.target.checked)} />} label="Keep checking until done" />
        {keepChecking && <Stack direction={{ xs: 'column', sm: 'row' }} spacing={2}>
          <TextField type="number" label="Follow-up delay (seconds)" value={delay} onChange={(e) => setDelay(Number(e.target.value))} fullWidth />
          <TextField type="number" label="Maximum wake-ups" value={maxWakeUps} onChange={(e) => setMaxWakeUps(Number(e.target.value))} fullWidth />
        </Stack>}
      </Stack>
    </DialogContent>
    <DialogActions><Button onClick={onClose}>Cancel</Button><Button variant="contained" onClick={submit} disabled={saving || !goal.trim() || steps.some((step) => !step.trim()) || !availability(agent) || (when === 'later' && !scheduledAt) || (when === 'repeat' && !cron.trim())}>{saving ? <CircularProgress size={18} /> : 'Create task'}</Button></DialogActions>
  </Dialog>;
}

function TaskSteps({ task }: { task: AgentTask }) {
  if (!task.steps?.length) return null;
  const outcomes = new Map((task.step_outcomes || []).map((outcome) => [outcome.step_id, outcome]));
  const current = task.current_step ?? 0;

  return <Box>
    <Stack direction="row" justifyContent="space-between" alignItems="baseline" sx={{ mb: 1 }}>
      <Typography variant="overline" color="text.secondary">Steps</Typography>
      <Typography variant="caption" color="text.secondary">{outcomes.size} of {task.steps.length} complete</Typography>
    </Stack>
    <Stack spacing={1}>
      {task.steps.map((step, index) => {
        const outcome = outcomes.get(step.id);
        const isCurrent = !outcome && index === current;
        const label = outcome ? 'Done' : isCurrent ? (task.status === 'needs_input' ? 'Needs you' : task.status === 'running' ? 'Working' : 'Current') : 'Next';
        const color = outcome ? 'success' : isCurrent ? (task.status === 'needs_input' ? 'warning' : 'primary') : 'default';
        return <Box key={step.id} sx={{ p: 1.5, border: '1px solid', borderColor: isCurrent ? 'primary.main' : 'divider', borderRadius: 1.5, bgcolor: isCurrent ? 'action.hover' : 'transparent' }}>
          <Stack direction="row" justifyContent="space-between" alignItems="center" gap={1}>
            <Typography variant="subtitle2">{index + 1}. {step.title}</Typography>
            <Chip size="small" label={label} color={color} variant={isCurrent ? 'filled' : 'outlined'} />
          </Stack>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5, whiteSpace: 'pre-wrap' }}>{step.instruction}</Typography>
          {outcome?.result.summary && <Typography variant="body2" sx={{ mt: 1 }}>{outcome.result.summary}</Typography>}
        </Box>;
      })}
    </Stack>
  </Box>;
}

function ControlRequestCard({ control, busy, onRespond }: {
  control: PendingControl; busy: boolean; onRespond: (decision: ControlDecision) => Promise<void>;
}) {
  const [answer, setAnswer] = useState('');
  useEffect(() => setAnswer(''), [control.id]);
  const input = control.input === undefined ? '' : typeof control.input === 'string' ? control.input : JSON.stringify(control.input, null, 2);
  const isQuestion = control.kind === 'question';

  return <Alert severity="warning" icon={<Security />} sx={{ '& .MuiAlert-message': { width: '100%' } }}>
    <Stack spacing={1.5}>
      <Box>
        <Typography variant="subtitle1">{isQuestion ? 'The agent needs an answer' : `${control.tool_name || 'A tool'} needs approval`}</Typography>
        <Typography variant="body2">{control.message || control.reason || (isQuestion ? 'Answer to continue this same run.' : 'Approve once to continue this same run.')}</Typography>
        <Typography variant="caption" color="text.secondary">Expires {formatTime(control.expires_at)}</Typography>
      </Box>
      {input && <Paper variant="outlined" sx={{ p: 1.25, bgcolor: 'background.paper', maxHeight: 180, overflow: 'auto' }}>
        <Typography component="pre" variant="body2" fontFamily="monospace" sx={{ m: 0, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>{input}</Typography>
      </Paper>}
      {isQuestion && <TextField size="small" multiline minRows={2} label="Your answer" value={answer} onChange={(event) => setAnswer(event.target.value)} />}
      <Stack direction="row" spacing={1} justifyContent="flex-end">
        <Button color="inherit" disabled={busy} onClick={() => onRespond({ action: 'deny', reason: 'Denied by user' })}>Deny</Button>
        <Button variant="contained" color="warning" startIcon={<Check />} disabled={busy || (isQuestion && !answer.trim())}
          onClick={() => onRespond(isQuestion ? { action: 'answer', answer: answer.trim() } : { action: 'approve' })}>
          {isQuestion ? 'Send answer' : 'Approve once'}
        </Button>
      </Stack>
    </Stack>
  </Alert>;
}

function ExecutionSummary({ task }: { task: AgentTask }) {
  const execution = task.execution || defaultExecution(task.agent);
  return <Box sx={{ minWidth: 200 }}>
    <Typography variant="overline" color="text.secondary">Access at launch</Typography>
    <Typography variant="body2">{getProfileMeta(execution.launch_profile).label}</Typography>
    <Typography variant="caption" color="text.secondary" fontFamily="monospace">{execution.launch_profile}</Typography>
    {execution.tools?.length ? <Typography variant="body2" sx={{ mt: 0.75 }}>{execution.tools.map((tool) => toolMeta[tool] || tool).join(' · ')}</Typography> : null}
  </Box>;
}

function RunHistory({ runs }: { runs: TaskRun[] }) {
  return <Box>
    <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 1.25 }}>
      <Stack direction="row" spacing={1} alignItems="center"><History fontSize="small" /><Typography variant="subtitle2">Run history</Typography></Stack>
      <Typography variant="caption" color="text.secondary">One record per bounded run</Typography>
    </Stack>
    {runs.length === 0 ? <Typography variant="body2" color="text.secondary">No run has started yet.</Typography> : <Stack spacing={1.25}>
      {runs.slice(0, 20).map((run, index) => {
        const meta = runStatusMeta[run.status];
        const title = run.trigger === 'step' ? `Step ${(run.step_index ?? 0) + 1}` : run.trigger === 'instruction' ? 'Instruction' : `Run ${runs.length - index}`;
        const summary = run.error || run.result?.summary || run.progress || run.instruction;
        return <Box key={run.id} sx={{ position: 'relative', pl: 2, pb: 0.25, borderLeft: '2px solid', borderColor: index === 0 && run.status.startsWith('waiting_') ? 'warning.main' : 'divider' }}>
          <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={0.5}>
            <Box>
              <Stack direction="row" spacing={1} alignItems="center"><Typography variant="subtitle2">{title}</Typography><Chip size="small" label={meta.label} color={meta.color} variant="outlined" /></Stack>
              <Typography variant="caption" color="text.secondary">{formatTime(run.started_at)} · {getProfileMeta(run.execution.launch_profile).label}</Typography>
            </Box>
            {run.finished_at && <Typography variant="caption" color="text.secondary">Finished {formatTime(run.finished_at)}</Typography>}
          </Stack>
          {summary && <Typography variant="body2" sx={{ mt: 0.75, whiteSpace: 'pre-wrap' }}>{summary}</Typography>}
          {run.events?.length ? <Stack spacing={0.25} sx={{ mt: 0.75 }}>
            {run.events.slice(-3).map((event) => <Typography key={event.id} variant="caption" color="text.secondary">{formatTime(event.created_at)} · {event.summary}</Typography>)}
          </Stack> : null}
        </Box>;
      })}
    </Stack>}
  </Box>;
}

function TaskDetail({ task, runs, onWake, onStop, onRespond }: {
  task: AgentTask; runs: TaskRun[]; onWake: (instruction?: string) => Promise<void>; onStop: () => Promise<void>;
  onRespond: (decision: ControlDecision) => Promise<void>;
}) {
  const [instructionOpen, setInstructionOpen] = useState(false);
  const [instruction, setInstruction] = useState('');
  const [busy, setBusy] = useState(false);
  const [actionError, setActionError] = useState('');
  const copy = (value?: string) => value && navigator.clipboard.writeText(value);
  const act = async (action: () => Promise<void>) => {
    setBusy(true); setActionError('');
    try { await action(); } catch (error) { setActionError((error as Error).message); } finally { setBusy(false); }
  };
  const send = async () => { await act(() => onWake(instruction)); setInstruction(''); setInstructionOpen(false); };

  return <Paper variant="outlined" sx={{ flex: 1, minWidth: 0, p: { xs: 2, md: 3 }, borderRadius: 2 }}>
    <Stack spacing={3}>
      {actionError && <Alert severity="error" onClose={() => setActionError('')}>{actionError}</Alert>}
      <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={2}>
        <Box><Stack direction="row" spacing={1} alignItems="center"><Typography variant="h4">{task.title || task.goal}</Typography><Chip size="small" label={statusMeta[task.status].label} color={statusMeta[task.status].color} /></Stack><Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>{task.goal}</Typography></Box>
        <Stack direction="row" spacing={1} flexWrap="wrap">
          {canStop(task) && <Button color="inherit" startIcon={<Block />} disabled={busy} onClick={() => act(onStop)}>Stop</Button>}
          <Button startIcon={<PlayArrow />} disabled={busy || task.status === 'running' || task.status === 'queued'} onClick={() => act(() => onWake())}>Run now</Button>
          <Button variant="contained" startIcon={<Send />} disabled={busy || task.status === 'running' || task.status === 'queued'} onClick={() => setInstructionOpen(true)}>Send instruction</Button>
        </Stack>
      </Stack>
      {task.attention && <ControlRequestCard control={task.attention} busy={busy} onRespond={(decision) => act(() => onRespond(decision))} />}
      {task.status === 'needs_input' && <Alert severity="warning"><Typography variant="subtitle2">Waiting for you</Typography>{task.latest_result?.question || 'The agent needs another instruction before it can continue.'}</Alert>}
      <TaskSteps task={task} />
      {(task.latest_result?.summary || task.error || task.progress) && <Box><Typography variant="overline" color="text.secondary">Latest outcome</Typography><Typography variant="body1" sx={{ whiteSpace: 'pre-wrap' }}>{task.error || task.latest_result?.summary || task.progress}</Typography></Box>}
      <Divider />
      <Stack direction={{ xs: 'column', md: 'row' }} spacing={4}>
        <Box sx={{ minWidth: 180 }}><Typography variant="overline" color="text.secondary">Execution</Typography><Typography variant="body2">Agent · {task.agent === 'claude' ? 'Claude Code' : 'Codex'}</Typography><Typography variant="body2">Wake-ups · {task.wake_count}</Typography><Typography variant="body2">Next run · {formatTime(task.scheduled_at)}</Typography></Box>
        <ExecutionSummary task={task} />
        <Box sx={{ minWidth: 0, flex: 1 }}><Typography variant="overline" color="text.secondary">Workspace</Typography><Stack direction="row" spacing={1} alignItems="center"><Typography variant="body2" fontFamily="monospace" noWrap>{task.workspace_path}</Typography><Tooltip title="Copy path"><Button size="small" onClick={() => copy(task.workspace_path)}><ContentCopy fontSize="small" /></Button></Tooltip></Stack>{task.session_id && <><Typography variant="overline" color="text.secondary" sx={{ mt: 1, display: 'block' }}>Native session</Typography><Typography variant="body2" fontFamily="monospace">{task.session_id}</Typography></>}</Box>
      </Stack>
      {task.resume_command && <Paper variant="outlined" sx={{ p: 1.5, bgcolor: 'action.hover' }}><Stack direction="row" justifyContent="space-between" alignItems="center" gap={2}><Typography variant="body2" fontFamily="monospace" sx={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{task.resume_command}</Typography><Button size="small" startIcon={<ContentCopy />} onClick={() => copy(task.resume_command)}>Copy</Button></Stack></Paper>}
      <Divider />
      <RunHistory runs={runs} />
    </Stack>
    <Dialog open={instructionOpen} onClose={() => setInstructionOpen(false)} maxWidth="sm" fullWidth><DialogTitle>Send instruction</DialogTitle><DialogContent><TextField autoFocus multiline minRows={3} fullWidth label="What should the agent know?" value={instruction} onChange={(e) => setInstruction(e.target.value)} sx={{ mt: 1 }} /></DialogContent><DialogActions><Button onClick={() => setInstructionOpen(false)}>Cancel</Button><Button variant="contained" disabled={!instruction.trim() || busy} onClick={send}>Send and run</Button></DialogActions></Dialog>
  </Paper>;
}

export default function TaskPage() {
  const { enableTasks } = useFeatureFlags();
  const [tasks, setTasks] = useState<AgentTask[]>([]); const [agents, setAgents] = useState<AgentAvailability[]>([]);
  const [selectedId, setSelectedId] = useState(''); const [loading, setLoading] = useState(true); const [error, setError] = useState(''); const [createOpen, setCreateOpen] = useState(false);
  const [detail, setDetail] = useState<AgentTask>(); const [runs, setRuns] = useState<TaskRun[]>([]);
  const load = useCallback(async (quiet = false) => { if (!quiet) setLoading(true); try { const [items, available] = await Promise.all([taskApi.list(), taskApi.agents()]); setTasks(items); setAgents(available); setSelectedId((current) => current && items.some((item) => item.id === current) ? current : items[0]?.id || ''); setError(''); } catch (err) { setError((err as Error).message); } finally { setLoading(false); } }, []);
  useEffect(() => { load(); const timer = window.setInterval(() => load(true), 5000); return () => window.clearInterval(timer); }, [load]);
  useEffect(() => {
    if (!selectedId) { setDetail(undefined); setRuns([]); return; }
    setRuns([]);
    let active = true;
    const refresh = async () => {
      try {
        const [task, history] = await Promise.all([taskApi.get(selectedId), taskApi.runs(selectedId)]);
        if (active) { setDetail(task); setRuns(history); }
      } catch (err) { if (active) setError((err as Error).message); }
    };
    refresh();
    const timer = window.setInterval(refresh, 2000);
    return () => { active = false; window.clearInterval(timer); };
  }, [selectedId]);
  const selected = detail?.id === selectedId ? detail : tasks.find((task) => task.id === selectedId);
  const groups = useMemo(() => [
    { label: 'Needs you', items: tasks.filter((task) => task.status === 'needs_input' || Boolean(task.attention)) },
    { label: 'Active & scheduled', items: tasks.filter((task) => isActive(task) && !task.attention) },
    { label: 'Completed', items: tasks.filter((task) => !isActive(task) && task.status !== 'needs_input' && !task.attention) },
  ].filter((group) => group.items.length), [tasks]);
  const update = (task: AgentTask) => { setTasks((items) => items.map((item) => item.id === task.id ? task : item)); setDetail(task); };
  const wake = async (instruction?: string) => { if (!selected) return; update(await taskApi.wake(selected.id, instruction)); };
  const stop = async () => { if (!selected) return; await taskApi.stop(selected.id); await load(true); };
  const respond = async (decision: ControlDecision) => {
    if (!selected?.attention || !selected.active_run_id) throw new Error('This request is no longer active.');
    await taskApi.respond(selected.id, selected.active_run_id, selected.attention.id, decision);
    const [task, history] = await Promise.all([taskApi.get(selected.id), taskApi.runs(selected.id)]);
    update(task); setRuns(history); await load(true);
  };

  return <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3, minHeight: '100%' }}>
    <PageHeader title="Tasks" subtitle="Give Claude Code or Codex a goal, then return when the work—or a decision—is ready." actions={<><Tooltip title="Refresh"><Button onClick={() => load()} startIcon={<Refresh />}>Refresh</Button></Tooltip><Button variant="contained" startIcon={<Add />} disabled={!enableTasks} onClick={() => setCreateOpen(true)}>New task</Button></>} />
    {!enableTasks && <Alert severity="info">Task creation is disabled. Existing tasks remain available so you can stop, inspect, or resume them.</Alert>}
    {error && <Alert severity="error">{error}</Alert>}
    {loading ? <Box sx={{ display: 'grid', placeItems: 'center', minHeight: 360 }}><CircularProgress /></Box> : tasks.length === 0 ? <Paper variant="outlined" sx={{ py: 10, textAlign: 'center', borderRadius: 2 }}><Schedule sx={{ fontSize: 40, color: 'text.secondary' }} /><Typography variant="h5" sx={{ mt: 2 }}>No tasks yet</Typography><Typography color="text.secondary" sx={{ mt: 1 }}>Create one goal. Tingly Box will handle the workspace, schedule, and native session.</Typography>{enableTasks && <Button variant="contained" startIcon={<Add />} sx={{ mt: 3 }} onClick={() => setCreateOpen(true)}>Create your first task</Button>}</Paper> : <Stack direction={{ xs: 'column', md: 'row' }} spacing={2} alignItems="stretch">
      <Paper variant="outlined" sx={{ width: { xs: '100%', md: 320, lg: 340 }, flexShrink: 0, p: 1.5, borderRadius: 2 }}><Stack spacing={2}>{groups.map((group) => <Box key={group.label}><Typography variant="overline" color="text.secondary" sx={{ px: 1 }}>{group.label}</Typography><Stack spacing={0.5}>{group.items.map((task) => { const meta = task.attention ? { label: 'Needs you', color: 'warning' as const } : statusMeta[task.status]; return <Box key={task.id} onClick={() => setSelectedId(task.id)} sx={{ p: 1.25, borderRadius: 1.5, cursor: 'pointer', bgcolor: selectedId === task.id ? 'action.selected' : 'transparent', border: '1px solid', borderColor: selectedId === task.id ? 'primary.main' : 'transparent', '&:hover': { bgcolor: 'action.hover' } }}><Stack direction="row" justifyContent="space-between" gap={1}><Typography variant="subtitle2" noWrap>{task.title || task.goal}</Typography><Chip size="small" label={meta.label} color={meta.color} /></Stack><Typography variant="caption" color="text.secondary">{task.agent === 'claude' ? 'Claude Code' : 'Codex'}{task.steps?.length ? ` · Step ${Math.min((task.current_step ?? 0) + 1, task.steps.length)}/${task.steps.length}` : ''} · {task.status === 'pending' ? formatTime(task.scheduled_at) : formatTime(task.updated_at)}</Typography></Box>; })}</Stack></Box>)}</Stack></Paper>
      {selected && <TaskDetail task={selected} runs={runs} onWake={wake} onStop={stop} onRespond={respond} />}
    </Stack>}
    <CreateTaskDialog open={createOpen} agents={agents} onClose={() => setCreateOpen(false)} onCreated={(task) => { setTasks((items) => [task, ...items]); setDetail(task); setRuns([]); setSelectedId(task.id); }} />
  </Box>;
}
