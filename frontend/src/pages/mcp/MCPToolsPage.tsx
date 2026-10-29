import { useState } from 'react';
import { useNavigate, useSearchParams } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import {
    Alert,
    Button,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    IconButton,
    Stack,
    TextField,
    ToggleButton,
    ToggleButtonGroup,
    Typography,
} from '@mui/material';
import { Add, Refresh } from '@/components/icons';
import { PageLayout } from '@/components/PageLayout';
import MCPSourceEditor from './MCPSourceEditor';
import MCPSourceDrawer from './MCPSourceDrawer';
import MCPToolsPanel from './MCPToolsPanel';
import MCPWorkspaceAlerts from './MCPWorkspaceAlerts';
import { nextMCPId } from './workspaceState';
import { useMCPWorkspace } from './useMCPWorkspace';
import { defaultMCPSourceFormValue, formValueToSource, githubSourceFormValue } from './types';
import { isServerToolSource } from './toolPresentation';

// Every connected MCP and its tools: connect any MCP, switch each tool globally.
// Which profile exposes a tool is decided on the Profile pages. Tingly Box's own
// Server Tools are not MCP and live on their own page.
export default function MCPToolsPage() {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const navigate = useNavigate();
    const [search, setSearch] = useSearchParams();
    const ws = useMCPWorkspace();
    const { config, routing, enabled, loading, checking, busy } = ws;
    const sources = (config.sources || []).filter((source) => !isServerToolSource(source));
    const [adding, setAdding] = useState(false);
    const [form, setForm] = useState(defaultMCPSourceFormValue);
    const [connectionError, setConnectionError] = useState('');
    // GitHub is a fixed, curated subset: a token is all it needs.
    const [github, setGithub] = useState(false);
    const [githubToken, setGithubToken] = useState('');
    const hasGitHub = sources.some((source) => source.preset === 'github');
    const selectedId = search.get('source');
    const closeSheet = () => {
        const next = new URLSearchParams(search);
        next.delete('source');
        setSearch(next, { replace: true });
    };
    const openSource = (id: string) => {
        const next = new URLSearchParams(search);
        next.set('source', id);
        setSearch(next);
    };
    const beginConnection = () => {
        setForm({
            ...defaultMCPSourceFormValue(),
            id: nextMCPId(
                'src',
                sources.map((s) => s.id!)
            ),
            transport: 'http',
        });
        setConnectionError('');
        setGithub(false);
        setGithubToken('');
        setAdding(true);
    };
    const pickGitHub = (on: boolean) => {
        setGithub(on);
        setConnectionError('');
        setForm(
            on
                ? githubSourceFormValue(form.id, githubToken)
                : { ...defaultMCPSourceFormValue(), id: form.id, transport: 'http' }
        );
    };
    return (
        <PageLayout
            loading={loading}
            title={label('toolsTitle', 'MCP Tools')}
            subtitle={label(
                'toolsHint',
                'Connect any MCP and manage its tools. A tool switched off here is off in every profile.'
            )}
            rightAction={
                <>
                    <IconButton
                        aria-label={label('refresh', 'Refresh connections')}
                        disabled={busy || checking}
                        onClick={() => void ws.run(ws.reload)}
                    >
                        <Refresh />
                    </IconButton>
                    <Button startIcon={<Add />} variant="contained" disabled={busy} onClick={beginConnection}>
                        {label('connectTools', 'Connect tools')}
                    </Button>
                </>
            }
        >
            <Stack spacing={2}>
                <MCPWorkspaceAlerts ws={ws} />
                <MCPToolsPanel
                    sources={sources}
                    routes={(routing?.sources || []).filter((route) => sources.some((source) => source.id === route.id))}
                    loading={checking}
                    enabled={enabled}
                    saveSource={ws.saveSource}
                    onConfigureSource={openSource}
                />
                <Button sx={{ alignSelf: 'flex-start' }} onClick={() => navigate('/mcp/profile/default')}>
                    {label('chooseProfileTools', 'Choose which tools each profile exposes')}
                </Button>
            </Stack>
            <MCPSourceDrawer ws={ws} selectedId={selectedId} onClose={closeSheet} />
            <Dialog open={adding} onClose={() => !busy && setAdding(false)} maxWidth="sm" fullWidth>
                <DialogTitle>{label('connectTools', 'Connect tools')}</DialogTitle>
                <DialogContent dividers>
                    <Stack spacing={2.5}>
                        <Typography variant="body2" color="text.secondary">
                            {label(
                                'connectDialogHint',
                                'Connect any MCP and we will discover its tools. They are added to the Default profile right away.'
                            )}
                        </Typography>
                        {connectionError && <Alert severity="error">{connectionError}</Alert>}
                        <ToggleButtonGroup exclusive fullWidth size="small" value={github ? 'github' : 'custom'} onChange={(_, v) => v && pickGitHub(v === 'github')}>
                            <ToggleButton value="custom">{label('connectCustom', 'Any MCP')}</ToggleButton>
                            <ToggleButton value="github" disabled={hasGitHub}>
                                GitHub
                            </ToggleButton>
                        </ToggleButtonGroup>
                        {github ? (
                            <>
                                <Alert severity="info">
                                    {label(
                                        'githubSubsetHint',
                                        'A fixed subset of the official GitHub MCP server: read repositories, issues, pull requests and Actions logs, plus comments, issues and pull requests. Deleting, merging, file writes, workflow runs and account-wide search are never offered. Limit it to specific repositories on a custom MCP profile.'
                                    )}
                                </Alert>
                                <TextField
                                    label={label('githubToken', 'GitHub personal access token')}
                                    type="password"
                                    value={githubToken}
                                    required
                                    autoComplete="off"
                                    onChange={(e) => {
                                        setGithubToken(e.target.value);
                                        setForm(githubSourceFormValue(form.id, e.target.value));
                                    }}
                                    helperText={label(
                                        'githubTokenHint',
                                        'Use a fine-grained token limited to the repositories you need.'
                                    )}
                                />
                            </>
                        ) : (
                            <>
                                <TextField
                                    label={label('connectionName', 'Connection name')}
                                    value={form.name}
                                    required
                                    onChange={(e) => setForm({ ...form, name: e.target.value })}
                                    placeholder={label('connectionNamePlaceholder', 'For example: Company knowledge base')}
                                />
                                <MCPSourceEditor compact hideEnabled hideTools value={form} onChange={setForm} />
                            </>
                        )}
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button disabled={busy} onClick={() => setAdding(false)}>
                        {label('cancel', 'Cancel')}
                    </Button>
                    <Button
                        variant="contained"
                        disabled={
                            busy ||
                            !form.name.trim() ||
                            !form.id.trim() ||
                            (github && !githubToken.trim()) ||
                            (form.transport === 'stdio' ? !form.command.trim() : !form.endpoint.trim())
                        }
                        onClick={() =>
                            void ws.run(async () => {
                                setConnectionError('');
                                try {
                                    if (sources.some((s) => s.id === form.id))
                                        throw new Error(label('duplicateId', 'This identifier is already in use.'));
                                    await ws.saveSource(formValueToSource(form));
                                    setAdding(false);
                                    openSource(form.id.trim());
                                } catch (e) {
                                    setConnectionError(e instanceof Error ? e.message : String(e));
                                    throw e;
                                }
                            })
                        }
                    >
                        {busy ? <CircularProgress size={18} /> : label('connectAndChoose', 'Connect and inspect tools')}
                    </Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
}
