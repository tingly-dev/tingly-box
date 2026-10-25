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
    Divider,
    Drawer,
    IconButton,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import { Add, Close, Refresh } from '@/components/icons';
import { PageLayout } from '@/components/PageLayout';
import MCPSourceEditor from './MCPSourceEditor';
import MCPSourceWorkspace from './MCPSourceWorkspace';
import MCPToolsPanel from './MCPToolsPanel';
import MCPWorkspaceAlerts from './MCPWorkspaceAlerts';
import { nextMCPId } from './workspaceState';
import { api } from '@/services/api';
import { useMCPWorkspace } from './useMCPWorkspace';
import { defaultMCPSourceFormValue, formValueToSource } from './types';

// Every tool in one place: connect any MCP, see its tools and Server Tools
// together, switch each one globally. Which profile exposes a tool is decided
// on the Profiles page.
export default function MCPToolsPage() {
    const { t } = useTranslation();
    const label = (key: string, fallback: string) => t(`mcp.workspace.${key}`, { defaultValue: fallback });
    const navigate = useNavigate();
    const [search, setSearch] = useSearchParams();
    const ws = useMCPWorkspace();
    const { config, routing, enabled, loading, checking, busy } = ws;
    const sources = config.sources || [];
    const [adding, setAdding] = useState(false);
    const [form, setForm] = useState(defaultMCPSourceFormValue);
    const [connectionError, setConnectionError] = useState('');
    const selectedId = search.get('source');
    const selected = selectedId ? sources.find((source) => source.id === selectedId) : undefined;
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
        setAdding(true);
    };
    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <Stack
                    direction="row"
                    sx={{ alignItems: 'center', justifyContent: 'space-between', gap: 2, flexWrap: 'wrap' }}
                >
                    <Stack>
                        <Typography component="h1" variant="h5" style={{ fontWeight: 700 }}>
                            {label('toolsTitle', 'All tools')}
                        </Typography>
                        <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                            {label(
                                'toolsHint',
                                'Connect any MCP and manage every tool, including the ones Tingly Box provides. A tool switched off here is off everywhere.'
                            )}
                        </Typography>
                    </Stack>
                    <Stack direction="row" spacing={1}>
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
                    </Stack>
                </Stack>
                <MCPWorkspaceAlerts ws={ws} />
                <MCPToolsPanel
                    sources={sources}
                    routes={routing?.sources || []}
                    loading={checking}
                    enabled={enabled}
                    saveSource={ws.saveSource}
                    onConfigureSource={openSource}
                />
                <Button sx={{ alignSelf: 'flex-start' }} onClick={() => navigate('/mcp/profile/default')}>
                    {label('chooseProfileTools', 'Choose which tools each profile exposes')}
                </Button>
            </Stack>
            <Drawer
                anchor="right"
                open={!!selectedId}
                onClose={closeSheet}
                slotProps={{
                    paper: {
                        role: 'dialog',
                        'aria-label': selected?.name || selected?.id || label('missingConnection', 'Connection'),
                        sx: { width: { xs: '100%', sm: 680 }, maxWidth: '100%' },
                    },
                }}
            >
                <Stack sx={{ p: 3 }} spacing={2.5}>
                    <Stack direction="row" sx={{ alignItems: 'center', justifyContent: 'space-between', gap: 1 }}>
                        <Typography variant="h5" style={{ fontWeight: 600 }}>
                            {selected?.name || selected?.id || label('missingConnection', 'Connection no longer exists')}
                        </Typography>
                        <IconButton aria-label={label('close', 'Close workspace panel')} onClick={closeSheet}>
                            <Close />
                        </IconButton>
                    </Stack>
                    <Divider />
                    {selected ? (
                        <MCPSourceWorkspace
                            key={selected.id}
                            source={selected}
                            route={routing?.sources.find((source) => source.id === selected.id)}
                            enabled={enabled}
                            saveSource={ws.saveSource}
                            onRefresh={async () => {
                                const response = await api.reconnectMCPSource(selected.id!);
                                await ws.refreshRoutes();
                                if (!response.success) throw new Error(response.error || 'Connection check failed');
                            }}
                            onDelete={async () => {
                                await ws.acceptConfig(await api.deleteMCPSource(selected.id!));
                                closeSheet();
                            }}
                        />
                    ) : (
                        <Alert severity="info">{label('missingConnection', 'Connection no longer exists')}</Alert>
                    )}
                </Stack>
            </Drawer>
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
                        <TextField
                            label={label('connectionName', 'Connection name')}
                            value={form.name}
                            required
                            onChange={(e) => setForm({ ...form, name: e.target.value })}
                            placeholder={label('connectionNamePlaceholder', 'For example: Company knowledge base')}
                        />
                        <MCPSourceEditor compact hideEnabled hideTools value={form} onChange={setForm} />
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
