import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  Alert, Box, Button, Chip, CircularProgress, Dialog, DialogActions, DialogContent, DialogTitle,
  Divider, FormControl, FormControlLabel, IconButton, InputLabel, MenuItem, Paper, Select, Stack, Switch,
  TextField, ToggleButton, ToggleButtonGroup, Tooltip, Typography,
} from '@mui/material';
import PageHeader from '@/components/PageHeader';
import { Add, Block, ContentCopy, Delete, PlayArrow, Refresh, Schedule, Send, Terminal } from '@/components/icons';
import { useFeatureFlags } from '@/contexts/FeatureFlagsContext';
import { taskApi } from '@/services/taskApi';
import type { AgentAvailability, AgentTask, CreateTaskInput, TaskAgent, TaskStatus } from './types';

const statusMeta: Record<TaskStatus, { label: string; color: 'default' | 'primary' | 'warning' | 'success' | 'error' }> = {
  pending: { label: 'Waiting', color: 'default' }, queued: { label: 'Queued', color: 'default' },
  running: { label: 'Working', color: 'primary' }, needs_input: { label: 'Needs you', color: 'warning' },
  succeeded: { label: 'Done', color: 'success' }, failed: { label: 'Failed', color: 'error' },
  cancelled: { label: 'Stopped', color: 'default' }, interrupted: { label: 'Interrupted', color: 'error' },
};

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
  const [when, setWhen] = useState<'now' | 'later' | 'repeat'>('now');
  const [scheduledAt, setScheduledAt] = useState('');
  const [cron, setCron] = useState('0 9 * * *');
  const [keepChecking, setKeepChecking] = useState(false);
  const [delay, setDelay] = useState(300);
  const [maxWakeUps, setMaxWakeUps] = useState(20);
  const [steps, setSteps] = useState<string[]>([]);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState('');
  const availability = (kind: TaskAgent) => agents.find((item) => item.agent === kind)?.available ?? false;

  useEffect(() => {
    if (availability(agent)) return;
    const firstAvailable = agents.find((item) => item.available);
    if (firstAvailable) setAgent(firstAvailable.agent);
  }, [agent, agents]);

  const submit = async () => {
    if (!goal.trim()) return;
    setSaving(true); setError('');
    const input: CreateTaskInput = {
      goal: goal.trim(), agent,
      follow_up: { enabled: keepChecking, delay_seconds: delay, max_wake_ups: maxWakeUps },
      timeout_seconds: 1800,
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
          <ToggleButtonGroup exclusive value={agent} onChange={(_, value) => value && setAgent(value)} fullWidth>
            {(['claude', 'codex'] as TaskAgent[]).map((kind) => <ToggleButton key={kind} value={kind} disabled={!availability(kind)} sx={{ textTransform: 'none' }}>
              <Stack direction="row" spacing={1} alignItems="center"><Terminal fontSize="small" /><span>{kind === 'claude' ? 'Claude Code' : 'Codex'}</span><Chip size="small" label={availability(kind) ? 'Available' : 'Not found'} color={availability(kind) ? 'success' : 'default'} variant="outlined" /></Stack>
            </ToggleButton>)}
          </ToggleButtonGroup>
        </Box>
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

function TaskDetail({ task, onWake, onStop }: { task: AgentTask; onWake: (instruction?: string) => Promise<void>; onStop: () => Promise<void> }) {
  const [instructionOpen, setInstructionOpen] = useState(false);
  const [instruction, setInstruction] = useState('');
  const [busy, setBusy] = useState(false);
  const copy = (value?: string) => value && navigator.clipboard.writeText(value);
  const act = async (action: () => Promise<void>) => { setBusy(true); try { await action(); } finally { setBusy(false); } };
  const send = async () => { await act(() => onWake(instruction)); setInstruction(''); setInstructionOpen(false); };

  return <Paper variant="outlined" sx={{ flex: 1, minWidth: 0, p: { xs: 2, md: 3 }, borderRadius: 2 }}>
    <Stack spacing={3}>
      <Stack direction={{ xs: 'column', sm: 'row' }} justifyContent="space-between" gap={2}>
        <Box><Stack direction="row" spacing={1} alignItems="center"><Typography variant="h4">{task.title || task.goal}</Typography><Chip size="small" label={statusMeta[task.status].label} color={statusMeta[task.status].color} /></Stack><Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>{task.goal}</Typography></Box>
        <Stack direction="row" spacing={1} flexWrap="wrap">
          {canStop(task) && <Button color="inherit" startIcon={<Block />} disabled={busy} onClick={() => act(onStop)}>Stop</Button>}
          <Button startIcon={<PlayArrow />} disabled={busy || task.status === 'running' || task.status === 'queued'} onClick={() => act(() => onWake())}>Run now</Button>
          <Button variant="contained" startIcon={<Send />} disabled={busy || task.status === 'running' || task.status === 'queued'} onClick={() => setInstructionOpen(true)}>Send instruction</Button>
        </Stack>
      </Stack>
      {task.status === 'needs_input' && <Alert severity="warning"><Typography variant="subtitle2">Waiting for you</Typography>{task.latest_result?.question || 'The agent needs another instruction before it can continue.'}</Alert>}
      <TaskSteps task={task} />
      {(task.latest_result?.summary || task.error || task.progress) && <Box><Typography variant="overline" color="text.secondary">Latest outcome</Typography><Typography variant="body1" sx={{ whiteSpace: 'pre-wrap' }}>{task.error || task.latest_result?.summary || task.progress}</Typography></Box>}
      <Divider />
      <Stack direction={{ xs: 'column', md: 'row' }} spacing={4}>
        <Box sx={{ minWidth: 180 }}><Typography variant="overline" color="text.secondary">Execution</Typography><Typography variant="body2">Agent · {task.agent === 'claude' ? 'Claude Code' : 'Codex'}</Typography><Typography variant="body2">Wake-ups · {task.wake_count}</Typography><Typography variant="body2">Next run · {formatTime(task.scheduled_at)}</Typography></Box>
        <Box sx={{ minWidth: 0, flex: 1 }}><Typography variant="overline" color="text.secondary">Workspace</Typography><Stack direction="row" spacing={1} alignItems="center"><Typography variant="body2" fontFamily="monospace" noWrap>{task.workspace_path}</Typography><Tooltip title="Copy path"><Button size="small" onClick={() => copy(task.workspace_path)}><ContentCopy fontSize="small" /></Button></Tooltip></Stack>{task.session_id && <><Typography variant="overline" color="text.secondary" sx={{ mt: 1, display: 'block' }}>Native session</Typography><Typography variant="body2" fontFamily="monospace">{task.session_id}</Typography></>}</Box>
      </Stack>
      {task.resume_command && <Paper variant="outlined" sx={{ p: 1.5, bgcolor: 'action.hover' }}><Stack direction="row" justifyContent="space-between" alignItems="center" gap={2}><Typography variant="body2" fontFamily="monospace" sx={{ overflow: 'hidden', textOverflow: 'ellipsis' }}>{task.resume_command}</Typography><Button size="small" startIcon={<ContentCopy />} onClick={() => copy(task.resume_command)}>Copy</Button></Stack></Paper>}
    </Stack>
    <Dialog open={instructionOpen} onClose={() => setInstructionOpen(false)} maxWidth="sm" fullWidth><DialogTitle>Send instruction</DialogTitle><DialogContent><TextField autoFocus multiline minRows={3} fullWidth label="What should the agent know?" value={instruction} onChange={(e) => setInstruction(e.target.value)} sx={{ mt: 1 }} /></DialogContent><DialogActions><Button onClick={() => setInstructionOpen(false)}>Cancel</Button><Button variant="contained" disabled={!instruction.trim() || busy} onClick={send}>Send and run</Button></DialogActions></Dialog>
  </Paper>;
}

export default function TaskPage() {
  const { enableTasks } = useFeatureFlags();
  const [tasks, setTasks] = useState<AgentTask[]>([]); const [agents, setAgents] = useState<AgentAvailability[]>([]);
  const [selectedId, setSelectedId] = useState(''); const [loading, setLoading] = useState(true); const [error, setError] = useState(''); const [createOpen, setCreateOpen] = useState(false);
  const load = useCallback(async (quiet = false) => { if (!quiet) setLoading(true); try { const [items, available] = await Promise.all([taskApi.list(), taskApi.agents()]); setTasks(items); setAgents(available); setSelectedId((current) => current && items.some((item) => item.id === current) ? current : items[0]?.id || ''); setError(''); } catch (err) { setError((err as Error).message); } finally { setLoading(false); } }, []);
  useEffect(() => { load(); const timer = window.setInterval(() => load(true), 5000); return () => window.clearInterval(timer); }, [load]);
  const selected = tasks.find((task) => task.id === selectedId);
  const groups = useMemo(() => [
    { label: 'Needs you', items: tasks.filter((task) => task.status === 'needs_input') },
    { label: 'Active & scheduled', items: tasks.filter(isActive) },
    { label: 'Completed', items: tasks.filter((task) => !isActive(task) && task.status !== 'needs_input') },
  ].filter((group) => group.items.length), [tasks]);
  const update = (task: AgentTask) => setTasks((items) => items.map((item) => item.id === task.id ? task : item));
  const wake = async (instruction?: string) => { if (!selected) return; update(await taskApi.wake(selected.id, instruction)); };
  const stop = async () => { if (!selected) return; await taskApi.stop(selected.id); await load(true); };

  return <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3, minHeight: '100%' }}>
    <PageHeader title="Tasks" subtitle="Give Claude Code or Codex a goal, then return when the work—or a decision—is ready." actions={<><Tooltip title="Refresh"><Button onClick={() => load()} startIcon={<Refresh />}>Refresh</Button></Tooltip><Button variant="contained" startIcon={<Add />} disabled={!enableTasks} onClick={() => setCreateOpen(true)}>New task</Button></>} />
    {!enableTasks && <Alert severity="info">Task creation is disabled. Existing tasks remain available so you can stop, inspect, or resume them.</Alert>}
    {error && <Alert severity="error">{error}</Alert>}
    {loading ? <Box sx={{ display: 'grid', placeItems: 'center', minHeight: 360 }}><CircularProgress /></Box> : tasks.length === 0 ? <Paper variant="outlined" sx={{ py: 10, textAlign: 'center', borderRadius: 2 }}><Schedule sx={{ fontSize: 40, color: 'text.secondary' }} /><Typography variant="h5" sx={{ mt: 2 }}>No tasks yet</Typography><Typography color="text.secondary" sx={{ mt: 1 }}>Create one goal. Tingly Box will handle the workspace, schedule, and native session.</Typography>{enableTasks && <Button variant="contained" startIcon={<Add />} sx={{ mt: 3 }} onClick={() => setCreateOpen(true)}>Create your first task</Button>}</Paper> : <Stack direction={{ xs: 'column', md: 'row' }} spacing={2} alignItems="stretch">
      <Paper variant="outlined" sx={{ width: { xs: '100%', md: 320, lg: 340 }, flexShrink: 0, p: 1.5, borderRadius: 2 }}><Stack spacing={2}>{groups.map((group) => <Box key={group.label}><Typography variant="overline" color="text.secondary" sx={{ px: 1 }}>{group.label}</Typography><Stack spacing={0.5}>{group.items.map((task) => <Box key={task.id} onClick={() => setSelectedId(task.id)} sx={{ p: 1.25, borderRadius: 1.5, cursor: 'pointer', bgcolor: selectedId === task.id ? 'action.selected' : 'transparent', border: '1px solid', borderColor: selectedId === task.id ? 'primary.main' : 'transparent', '&:hover': { bgcolor: 'action.hover' } }}><Stack direction="row" justifyContent="space-between" gap={1}><Typography variant="subtitle2" noWrap>{task.title || task.goal}</Typography><Chip size="small" label={statusMeta[task.status].label} color={statusMeta[task.status].color} /></Stack><Typography variant="caption" color="text.secondary">{task.agent === 'claude' ? 'Claude Code' : 'Codex'}{task.steps?.length ? ` · Step ${Math.min((task.current_step ?? 0) + 1, task.steps.length)}/${task.steps.length}` : ''} · {task.status === 'pending' ? formatTime(task.scheduled_at) : formatTime(task.updated_at)}</Typography></Box>)}</Stack></Box>)}</Stack></Paper>
      {selected && <TaskDetail task={selected} onWake={wake} onStop={stop} />}
    </Stack>}
    <CreateTaskDialog open={createOpen} agents={agents} onClose={() => setCreateOpen(false)} onCreated={(task) => { setTasks((items) => [task, ...items]); setSelectedId(task.id); }} />
  </Box>;
}
