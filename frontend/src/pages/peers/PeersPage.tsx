import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import EmptyState from '@/components/EmptyState';
import { api } from '@/services/api';
import type { BotChat, BotSettings } from '@/types/bot';
import type { Peer } from '@/types/peer';
import { useNotify } from '@/hooks/useNotify';
import {
    Add as IconPlus,
    Autorenew as IconRotate,
    Cable as IconCable,
    ContentCopy as IconCopy,
    Delete as IconTrash,
    Edit as IconEdit,
    FiberManualRecord as IconDot,
} from '@/components/icons';
import {
    Autocomplete,
    Box,
    Button,
    Checkbox,
    Chip,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControlLabel,
    IconButton,
    MenuItem,
    Stack,
    Switch,
    Table,
    TableBody,
    TableCell,
    TableHead,
    TableRow,
    TextField,
    Tooltip,
    Typography,
} from '@mui/material';
import { useCallback, useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';

// A Peer is an external tool that talks through one of your bots: it holds a
// scoped tb-peer- token and speaks two verbs (send / updates) against
// tingly-box as if tingly-box were the IM platform. See .design/peer.md.

const NAME_RE = /^[a-z0-9_-]{2,32}$/;

interface PeerForm {
    name: string;
    bot_uuid: string;
    chat_id: string;
    exclusive: boolean;
    enabled: boolean;
}

const emptyForm: PeerForm = { name: '', bot_uuid: '', chat_id: '', exclusive: false, enabled: true };

const PeersPage = () => {
    const { t } = useTranslation();
    const notify = useNotify();

    const [peers, setPeers] = useState<Peer[]>([]);
    const [bots, setBots] = useState<BotSettings[]>([]);
    const [loading, setLoading] = useState(true);

    // Create / edit dialog (one form, mode decided by editUuid).
    const [formOpen, setFormOpen] = useState(false);
    const [editUuid, setEditUuid] = useState<string | null>(null);
    const [form, setForm] = useState<PeerForm>(emptyForm);
    const [saving, setSaving] = useState(false);

    // One-time token dialog (create + rotate land here).
    const [tokenPeer, setTokenPeer] = useState<Peer | null>(null);
    const [token, setToken] = useState('');

    // Delete confirm.
    const [peerToDelete, setPeerToDelete] = useState<Peer | null>(null);
    const [deleting, setDeleting] = useState(false);

    // Chats the selected bot has seen — the picker options for the binding.
    const [chatOptions, setChatOptions] = useState<BotChat[]>([]);
    const [chatsLoading, setChatsLoading] = useState(false);

    useEffect(() => {
        if (!formOpen || !form.bot_uuid) {
            setChatOptions([]);
            return;
        }
        let cancelled = false;
        setChatsLoading(true);
        api.listBotChats(form.bot_uuid)
            .then((res) => {
                if (!cancelled) setChatOptions((res.chats || []).filter((c) => c.chat_id && !c.blocked));
            })
            .catch(() => {
                if (!cancelled) setChatOptions([]);
            })
            .finally(() => {
                if (!cancelled) setChatsLoading(false);
            });
        return () => {
            cancelled = true;
        };
    }, [formOpen, form.bot_uuid]);

    const botName = useMemo(() => {
        const byUuid = new Map(bots.map((b) => [b.uuid, b.name || b.platform || b.uuid]));
        return (uuid: string) => byUuid.get(uuid) || uuid;
    }, [bots]);

    const load = useCallback(async () => {
        try {
            const [peerResult, botResult] = await Promise.all([
                api.listPeers(),
                api.getImBotSettingsList(),
            ]);
            setPeers(peerResult.peers || []);
            if (botResult?.success && Array.isArray(botResult.settings)) {
                setBots(botResult.settings);
            }
        } catch (err: any) {
            notify.error(err.message || t('peers.loadFailed', { defaultValue: 'Failed to load peers' }));
        } finally {
            setLoading(false);
        }
    }, [notify, t]);

    useEffect(() => {
        load();
    }, [load]);

    const openCreate = () => {
        setEditUuid(null);
        setForm({ ...emptyForm, bot_uuid: bots[0]?.uuid || '' });
        setFormOpen(true);
    };

    const openEdit = (p: Peer) => {
        setEditUuid(p.uuid);
        setForm({ name: p.name, bot_uuid: p.bot_uuid, chat_id: p.chat_id, exclusive: p.exclusive, enabled: p.enabled });
        setFormOpen(true);
    };

    const nameInvalid = form.name !== '' && !NAME_RE.test(form.name);
    const formIncomplete = !form.name || !form.bot_uuid || !form.chat_id || nameInvalid;

    const handleSave = async () => {
        setSaving(true);
        try {
            if (editUuid) {
                await api.updatePeer(editUuid, form);
                notify.success(t('peers.updated', { defaultValue: 'Peer updated' }));
            } else {
                const result = await api.createPeer(form);
                // Surface the one-time token immediately — it cannot be
                // recovered later, only rotated.
                setTokenPeer(result.peer);
                setToken(result.token);
                notify.success(t('peers.created', { defaultValue: 'Peer registered' }));
            }
            setFormOpen(false);
            load();
        } catch (err: any) {
            notify.error(err.message || t('peers.saveFailed', { defaultValue: 'Failed to save peer' }));
        } finally {
            setSaving(false);
        }
    };

    const handleToggleEnabled = async (p: Peer) => {
        try {
            await api.updatePeer(p.uuid, { enabled: !p.enabled });
            load();
        } catch (err: any) {
            notify.error(err.message);
        }
    };

    const handleRotate = async (p: Peer) => {
        try {
            const result = await api.rotatePeerToken(p.uuid);
            setTokenPeer(p);
            setToken(result.token);
        } catch (err: any) {
            notify.error(err.message);
        }
    };

    const handleDelete = async () => {
        if (!peerToDelete) return;
        setDeleting(true);
        try {
            await api.deletePeer(peerToDelete.uuid);
            notify.success(t('peers.deleted', { defaultValue: 'Peer deleted' }));
            setPeerToDelete(null);
            load();
        } catch (err: any) {
            notify.error(err.message);
        } finally {
            setDeleting(false);
        }
    };

    const copy = (text: string) => {
        navigator.clipboard.writeText(text);
        notify.success(t('common.copied', { defaultValue: 'Copied to clipboard' }));
    };

    // The artifact for the next action: a working getUpdates call with the
    // real values filled in, ready to paste into the tool.
    const quickStart = tokenPeer
        ? `curl -H "Authorization: Bearer ${token}" \\\n  "http://127.0.0.1:12580/api/v1/peers/${tokenPeer.uuid}/updates?timeout=25s"`
        : '';

    return (
        <PageLayout loading={loading}>
            <Stack spacing={3}>
                <UnifiedCard
                    title={t('peers.title', { defaultValue: 'Peers' })}
                    titleHeadingLevel={1}
                    subtitle={t('peers.subtitle', {
                        defaultValue:
                            'External tools that talk through your bots. A peer gets a scoped token and two verbs — send and updates — as if Tingly Box were its IM platform. In chat, reach it with @name.',
                    })}
                    size="full"
                    rightAction={
                        <Button
                            variant="contained"
                            startIcon={<IconPlus sx={{ fontSize: 18 }} />}
                            onClick={openCreate}
                        >
                            {t('peers.register', { defaultValue: 'Register Peer' })}
                        </Button>
                    }
                >
                    {peers.length === 0 ? (
                        <EmptyState
                            icon={<IconCable sx={{ fontSize: 42 }} />}
                            title={t('peers.emptyTitle', { defaultValue: 'No peers yet' })}
                            description={t('peers.emptyDesc', {
                                defaultValue:
                                    'Register a peer to give a cron job, CI gate, or any external script a two-way line into one of your chats.',
                            })}
                        />
                    ) : (
                        <Box sx={{ overflowX: 'auto' }}>
                            <Table size="small">
                                <TableHead>
                                    <TableRow>
                                        <TableCell>{t('peers.colName', { defaultValue: 'Name' })}</TableCell>
                                        <TableCell>{t('peers.colBot', { defaultValue: 'Bot' })}</TableCell>
                                        <TableCell>{t('peers.colChat', { defaultValue: 'Chat' })}</TableCell>
                                        <TableCell>{t('peers.colMode', { defaultValue: 'Mode' })}</TableCell>
                                        <TableCell>{t('peers.colEnabled', { defaultValue: 'Enabled' })}</TableCell>
                                        <TableCell align="right">{t('peers.colActions', { defaultValue: 'Actions' })}</TableCell>
                                    </TableRow>
                                </TableHead>
                                <TableBody>
                                    {peers.map((p) => (
                                        <TableRow key={p.uuid} hover>
                                            <TableCell>
                                                <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                                                    <Tooltip
                                                        title={p.online
                                                            ? t('peers.online', { defaultValue: 'Connected — a poller is waiting for updates' })
                                                            : t('peers.offline', { defaultValue: 'Offline — messages are queued until it next polls' })}
                                                    >
                                                        <IconDot sx={{ fontSize: 12 }} color={p.online ? 'success' : 'disabled'} />
                                                    </Tooltip>
                                                    <Typography variant="body2" sx={{ fontWeight: 600 }}>@{p.name}</Typography>
                                                </Stack>
                                            </TableCell>
                                            <TableCell>{botName(p.bot_uuid)}</TableCell>
                                            <TableCell>
                                                <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{p.chat_id}</Typography>
                                            </TableCell>
                                            <TableCell>
                                                {p.exclusive ? (
                                                    <Tooltip title={t('peers.exclusiveHint', { defaultValue: 'Every plain message in the bound chat goes to this peer' })}>
                                                        <Chip size="small" label={t('peers.exclusive', { defaultValue: 'exclusive' })} color="primary" variant="outlined" />
                                                    </Tooltip>
                                                ) : (
                                                    <Tooltip title={t('peers.sharedHint', { defaultValue: 'Reach it with @name, by replying to its messages, or sticky after a handoff' })}>
                                                        <Chip size="small" label={t('peers.shared', { defaultValue: 'shared' })} variant="outlined" />
                                                    </Tooltip>
                                                )}
                                            </TableCell>
                                            <TableCell>
                                                <Switch size="small" checked={p.enabled} onChange={() => handleToggleEnabled(p)} />
                                            </TableCell>
                                            <TableCell align="right">
                                                <Tooltip title={t('peers.rotate', { defaultValue: 'Rotate token (old one stops working immediately)' })}>
                                                    <IconButton size="small" onClick={() => handleRotate(p)}>
                                                        <IconRotate sx={{ fontSize: 18 }} />
                                                    </IconButton>
                                                </Tooltip>
                                                <Tooltip title={t('common.edit', { defaultValue: 'Edit' })}>
                                                    <IconButton size="small" onClick={() => openEdit(p)}>
                                                        <IconEdit sx={{ fontSize: 18 }} />
                                                    </IconButton>
                                                </Tooltip>
                                                <Tooltip title={t('common.delete', { defaultValue: 'Delete' })}>
                                                    <IconButton size="small" color="error" onClick={() => setPeerToDelete(p)}>
                                                        <IconTrash sx={{ fontSize: 18 }} />
                                                    </IconButton>
                                                </Tooltip>
                                            </TableCell>
                                        </TableRow>
                                    ))}
                                </TableBody>
                            </Table>
                        </Box>
                    )}
                </UnifiedCard>
            </Stack>

            {/* Create / edit dialog */}
            <Dialog open={formOpen} onClose={() => setFormOpen(false)} maxWidth="sm" fullWidth>
                <DialogTitle>
                    {editUuid
                        ? t('peers.editTitle', { defaultValue: 'Edit Peer' })
                        : t('peers.createTitle', { defaultValue: 'Register Peer' })}
                </DialogTitle>
                <DialogContent>
                    <Stack spacing={3} sx={{ mt: 1 }}>
                        <TextField
                            label={t('peers.formName', { defaultValue: 'Name' })}
                            fullWidth
                            value={form.name}
                            onChange={(e) => setForm({ ...form, name: e.target.value })}
                            placeholder="report"
                            error={nameInvalid}
                            helperText={t('peers.formNameHelp', {
                                defaultValue: 'The mention word: @name in chat, 【name】 on its messages. 2–32 chars of a-z 0-9 _ -',
                            })}
                            autoFocus={!editUuid}
                        />
                        <TextField
                            select
                            label={t('peers.formBot', { defaultValue: 'Bot' })}
                            fullWidth
                            value={form.bot_uuid}
                            onChange={(e) => setForm({ ...form, bot_uuid: e.target.value })}
                            helperText={t('peers.formBotHelp', { defaultValue: 'The bot whose channel this peer talks through' })}
                        >
                            {bots.map((b) => (
                                <MenuItem key={b.uuid} value={b.uuid!}>
                                    {b.name || b.platform} <Typography component="span" variant="caption" sx={{ ml: 1, color: 'text.secondary' }}>{b.platform}</Typography>
                                </MenuItem>
                            ))}
                        </TextField>
                        <Autocomplete
                            freeSolo
                            options={chatOptions}
                            loading={chatsLoading}
                            getOptionLabel={(option) => (typeof option === 'string' ? option : option.chat_id)}
                            inputValue={form.chat_id}
                            onInputChange={(_, value) => setForm({ ...form, chat_id: value })}
                            renderOption={(props, option) => (
                                <Box component="li" {...props} key={option.id}>
                                    <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                                        <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{option.chat_id}</Typography>
                                        {option.is_paired && (
                                            <Chip size="small" variant="outlined" color="success"
                                                label={t('peers.paired', { defaultValue: 'paired' })} />
                                        )}
                                    </Stack>
                                </Box>
                            )}
                            renderInput={(params) => (
                                <TextField
                                    {...params}
                                    label={t('peers.formChat', { defaultValue: 'Chat' })}
                                    helperText={
                                        chatOptions.length > 0
                                            ? t('peers.formChatHelpPick', {
                                                defaultValue:
                                                    'Pick one of the chats this bot has seen. The binding is the authorization — the peer can never reach any other chat.',
                                            })
                                            : t('peers.formChatHelpManual', {
                                                defaultValue:
                                                    'No chats recorded for this bot yet — message the bot once so the chat appears here, or paste the chat id manually. The binding is the authorization.',
                                            })
                                    }
                                />
                            )}
                        />
                        <FormControlLabel
                            control={
                                <Checkbox
                                    checked={form.exclusive}
                                    onChange={(e) => setForm({ ...form, exclusive: e.target.checked })}
                                />
                            }
                            label={
                                <Box>
                                    <Typography variant="body2">{t('peers.formExclusive', { defaultValue: 'Exclusive chat' })}</Typography>
                                    <Typography variant="caption" color="text.secondary">
                                        {t('peers.formExclusiveHelp', {
                                            defaultValue: 'Every plain message in the chat goes to this peer — for a dedicated chat. Leave off to share the chat with agents and address it by @name.',
                                        })}
                                    </Typography>
                                </Box>
                            }
                        />
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setFormOpen(false)}>{t('common.cancel', { defaultValue: 'Cancel' })}</Button>
                    <Button
                        variant="contained"
                        onClick={handleSave}
                        disabled={saving || formIncomplete}
                        startIcon={saving ? <CircularProgress size={16} /> : undefined}
                    >
                        {editUuid
                            ? t('common.save', { defaultValue: 'Save' })
                            : t('peers.register', { defaultValue: 'Register Peer' })}
                    </Button>
                </DialogActions>
            </Dialog>

            {/* One-time token dialog */}
            <Dialog open={!!tokenPeer} onClose={() => setTokenPeer(null)} maxWidth="sm" fullWidth>
                <DialogTitle>{t('peers.tokenTitle', { defaultValue: 'Peer token — shown only once' })}</DialogTitle>
                <DialogContent>
                    <Stack spacing={2} sx={{ mt: 1 }}>
                        <Typography variant="body2" color="text.secondary">
                            {t('peers.tokenHelp', {
                                defaultValue:
                                    'Give this token to the tool. It only works on this peer’s send/updates endpoints — never the control plane. If it leaks or is lost, rotate it.',
                            })}
                        </Typography>
                        <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                            <TextField
                                fullWidth
                                size="small"
                                value={token}
                                slotProps={{ input: { readOnly: true, sx: { fontFamily: 'monospace' } } }}
                            />
                            <Tooltip title={t('common.copy', { defaultValue: 'Copy' })}>
                                <IconButton onClick={() => copy(token)}>
                                    <IconCopy sx={{ fontSize: 18 }} />
                                </IconButton>
                            </Tooltip>
                        </Stack>
                        <Typography variant="body2" color="text.secondary">
                            {t('peers.quickStart', { defaultValue: 'Try it — long-poll the inbound stream:' })}
                        </Typography>
                        <Box
                            component="pre"
                            sx={{
                                m: 0,
                                p: 1.5,
                                borderRadius: 1,
                                bgcolor: 'action.hover',
                                fontFamily: 'monospace',
                                fontSize: 12,
                                overflowX: 'auto',
                                whiteSpace: 'pre',
                            }}
                        >
                            {quickStart}
                        </Box>
                    </Stack>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => copy(token)} startIcon={<IconCopy sx={{ fontSize: 16 }} />}>
                        {t('common.copy', { defaultValue: 'Copy' })}
                    </Button>
                    <Button variant="contained" onClick={() => setTokenPeer(null)}>
                        {t('peers.tokenDone', { defaultValue: 'I saved it' })}
                    </Button>
                </DialogActions>
            </Dialog>

            {/* Delete confirm */}
            <Dialog open={!!peerToDelete} onClose={() => setPeerToDelete(null)} maxWidth="sm" fullWidth>
                <DialogTitle>
                    <Stack direction="row" spacing={1} sx={{ alignItems: 'center' }}>
                        <IconTrash color="error" />
                        <span>{t('peers.deleteTitle', { defaultValue: 'Delete Peer' })}</span>
                    </Stack>
                </DialogTitle>
                <DialogContent>
                    <Typography>
                        {t('peers.deleteConfirm', {
                            defaultValue: 'Delete @{{name}}? Its token stops working and queued updates are dropped. This cannot be undone.',
                            name: peerToDelete?.name,
                        })}
                    </Typography>
                </DialogContent>
                <DialogActions>
                    <Button onClick={() => setPeerToDelete(null)} disabled={deleting}>
                        {t('common.cancel', { defaultValue: 'Cancel' })}
                    </Button>
                    <Button
                        variant="contained"
                        color="error"
                        onClick={handleDelete}
                        disabled={deleting}
                        startIcon={deleting ? <CircularProgress size={16} /> : <IconTrash sx={{ fontSize: 18 }} />}
                    >
                        {t('common.delete', { defaultValue: 'Delete' })}
                    </Button>
                </DialogActions>
            </Dialog>
        </PageLayout>
    );
};

export default PeersPage;
