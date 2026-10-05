import {ContentCopy as CopyIcon, Edit as CustomIcon, Close as CloseIcon, Code as CodeIcon, Refresh as RefreshIcon, Block as BlockIcon, Delete as DeleteIcon} from '@/components/icons';
import {api} from '@/services/api';
import {notify} from '@/utils/notify';
import {capabilityEnabled, isPairingRequired} from '@/types/bot';
import type {BotGroupDetail, BotSettings, DirectChatDetail, NotifyTarget} from '@/types/bot';
import { fontMono, fontSizes } from '@/theme/fonts';
import NotifyTestDialog from '@/components/notify/NotifyTestDialog';
import ConfirmDialog from '@/components/ConfirmDialog';
import {CHAT_CAPABILITIES} from '@/components/notify/chatCapabilities';
import useChatProbe, {type ChatCapability, type ChatProbeResult} from '@/components/notify/useChatProbe';
import {ApiEntryNode, ArrowNode, ChatNode, ImBotNode, NodeContainer, graphRowStyles} from '@/components/nodes';
import {
    Alert,
    Box,
    Button,
    Chip,
    CircularProgress,
    Collapse,
    IconButton,
    Link,
    Stack,
    Switch,
    Tooltip,
    Typography,
} from '@mui/material';
import {useCallback, useEffect, useState} from 'react';
import {PLATFORM_BRAND_ICONS} from '@/constants/platformGuides';
import {useTranslation} from 'react-i18next';

// BotNotifyGroup is one bot's panel on the IM Notify page: a header (name +
// platform + the enabled switch that governs whether this bot can be driven)
// over an ALWAYS-EXPANDED list of the chats it can reach. Each chat row is a
// CAPABILITY PROBE BENCH — a row of one-click buttons (Notify / Confirm active,
// Choose / Ask gated, Custom → free-form dialog) that exercise each chat
// capability end-to-end and show a probe-style verdict inline, exactly as the
// model-routing probe does for providers (see components/probe/). This answers
// the operator's real question — "do my bot's chat capabilities actually work?"
// — not "can I compose one custom message?" (ux-principles #1/#5/#11).
//
// The chats are fetched eagerly when the bot is enabled, not behind a button.
// A disabled bot has no channel in the registry, so /chats would answer an
// empty list with running:false — we skip the round trip entirely and let the
// graph's empty fork explain why (the "Bot is off" leaf text).
export interface BotNotifyGroupProps {
    bot: BotSettings;
    onToggle: (uuid: string, enabled: boolean) => void;
    isToggling?: boolean;
}

// formatLatency mirrors probe/runProbe.ts: "850ms" / "1.2s".
const formatLatency = (ms: number): string => (ms < 1000 ? `${ms}ms` : `${(ms / 1000).toFixed(1)}s`);

// One-glance verdict label + severity for a probe result, mirroring the probe
// feature's StatusBar mapping of outcome → label.
const verdict = (r: ChatProbeResult): {label: string; severity: 'success' | 'error' | 'warning' | 'info'} => {
    switch (r.status) {
        case 'delivered': return {label: `Delivered · ${formatLatency(r.latencyMs)}`, severity: 'success'};
        case 'answered': return {label: `Answered: ${String(r.decision?.selected ?? '?')} · ${formatLatency(r.latencyMs)}`, severity: 'success'};
        case 'cancelled': return {label: `Cancelled · ${formatLatency(r.latencyMs)}`, severity: 'warning'};
        case 'timed-out': return {label: `Timed out · ${formatLatency(r.latencyMs)}`, severity: 'warning'};
        case 'expired': return {label: `Expired · ${formatLatency(r.latencyMs)}`, severity: 'warning'};
        default: return {label: r.error ? `Failed: ${r.error}` : 'Failed', severity: 'error'};
    }
};

// ProbeResultLine is the inline verdict for one capability probe — an outlined
// Alert (mirroring probe's StatusBar) with the capability, the one-glance
// outcome, and a collapsible Raw JSON. Dismissible so the row stays clean.
const ProbeResultLine: React.FC<{result: ChatProbeResult; onDismiss: () => void}> = ({result, onDismiss}) => {
    const {t} = useTranslation();
    const [showRaw, setShowRaw] = useState(false);
    const v = verdict(result);
    return (
        <Alert
            severity={v.severity}
            variant="outlined"
            icon={false}
            sx={{py: 0.5, borderRadius: 1, '& .MuiAlert-message': {width: '100%'}}}
        >
            <Box sx={{display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap'}}>
                <Chip label={result.capability} size="small" sx={{textTransform: 'capitalize', fontWeight: 600}} />
                <Typography variant="body2" sx={{fontWeight: 600, color: v.severity === 'success' ? 'success.main' : v.severity === 'error' ? 'error.main' : 'warning.main'}}>
                    {v.label}
                </Typography>
                {result.reason && (
                    <Typography variant="caption" sx={{color: 'text.secondary'}}>{result.reason}</Typography>
                )}
                <Box sx={{flexGrow: 1}} />
                <Tooltip title={t('notify.probe.showRaw', {defaultValue: 'Show raw payload'})}>
                    <IconButton size="small" onClick={() => setShowRaw((s) => !s)}>
                        <CodeIcon fontSize="small" />
                    </IconButton>
                </Tooltip>
                <Tooltip title={t('common.dismiss', {defaultValue: 'Dismiss'})}>
                    <IconButton size="small" onClick={onDismiss}>
                        <CloseIcon fontSize="small" />
                    </IconButton>
                </Tooltip>
            </Box>
            <Collapse in={showRaw}>
                <Box sx={{mt: 1, p: 1, bgcolor: 'action.hover', borderRadius: 1, fontFamily: fontMono, fontSize: fontSizes.sm, whiteSpace: 'pre-wrap', wordBreak: 'break-all'}}>
                    {JSON.stringify(result.raw ?? result, null, 2)}
                </Box>
            </Collapse>
        </Alert>
    );
};



const BotNotifyGroup: React.FC<BotNotifyGroupProps> = ({bot, onToggle, isToggling}) => {
    const {t} = useTranslation();
    const capabilityOn = capabilityEnabled(bot, 'notify');
    const enabled = (bot.enabled ?? true) && capabilityOn;

    const [targets, setTargets] = useState<NotifyTarget[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [testTargetID, setTestTargetID] = useState<string | null>(null);
    const [showDisabled, setShowDisabled] = useState(false);
    const [busyTarget, setBusyTarget] = useState<string | null>(null);
    const [deleteTarget, setDeleteTarget] = useState<string | null>(null);

    const loadChats = useCallback(async () => {
        if (!bot.uuid) return;
        setLoading(true);
        setError(null);
        try {
            const [chatData, groupData] = await Promise.all([
                api.listBotDirectChats(bot.uuid),
                api.listBotGroups(bot.uuid),
            ]);
            const groupDetails: BotGroupDetail[] = await Promise.all(
                (groupData.groups || []).map((group: {id: string}) => api.getBotGroup(bot.uuid!, group.id)),
            );
            const directTargets: NotifyTarget[] = (chatData.chats || []).map((detail: DirectChatDetail) => ({
                id: detail.chat.id,
                kind: 'direct_chat',
                external_id: detail.chat.external_chat_id,
                platform: detail.chat.platform,
                is_paired: Boolean(detail.chat.peer_actor_id),
                blocked: detail.chat.blocked,
                can_notify: detail.permissions.some((permission) => permission.capability === 'notify' && permission.action === 'notify.receive' && permission.effect === 'allow'),
                can_reply: detail.permissions.some((permission) => permission.capability === 'notify' && permission.action === 'notify.reply' && permission.effect === 'allow'),
            }));
            const groupTargets: NotifyTarget[] = groupDetails.map((detail) => ({
                id: detail.group.id,
                kind: 'group',
                external_id: detail.group.external_group_id,
                name: detail.group.name,
                platform: detail.group.platform,
                blocked: detail.group.blocked,
                can_notify: detail.capabilities.notify === 'allow',
                // Group replies are actor-scoped. Keep Confirm on Direct Chats
                // until the UI can select and explain the replying Actor.
                can_reply: false,
            }));
            setTargets([...directTargets, ...groupTargets]);
        } catch (loadError) {
            setError((loadError as Error).message);
        } finally {
            setLoading(false);
        }
    }, [bot.uuid]);

    // Eager-load only when the bot is enabled (a stopped bot has no reachable
    // chats). Re-fetch on enable transitions so toggling on surfaces fresh chats.
    useEffect(() => {
        if (enabled) loadChats();
        else {
            setTargets([]);
            setError(null);
            // A fetch may still be in flight from before the toggle — clear
            // loading so the graph (with its disabled body) renders instead of
            // a spinner that only resolves when the stale response lands.
            setLoading(false);
        }
    }, [enabled, loadChats]);

    const handleCopy = useCallback(async (targetID: string) => {
        try {
            await navigator.clipboard.writeText(targetID);
            notify.success(t('notify.chat.copied', {defaultValue: 'Target UUID copied'}));
        } catch {
            notify.error(t('notify.chat.copyFailed', {defaultValue: 'Copy failed — check clipboard permissions'}));
        }
    }, [t]);

    const openTest = useCallback((targetID: string) => setTestTargetID(targetID), []);
    const closeTest = useCallback(() => setTestTargetID(null), []);

    // Chat lifecycle actions — disable (inbound blocklist; the chat also drops
    // out of notify/interact) and hard delete (record removed; the chat
    // re-registers fresh if it messages the bot again).
    const handleToggleDisabled = useCallback(async (target: NotifyTarget) => {
        if (!bot.uuid) return;
        const blocked = !target.blocked;
        setBusyTarget(target.id);
        try {
            if (target.kind === 'group') {
                await api.setBotGroupBlocked(bot.uuid, target.id, blocked);
            } else {
                await api.setBotDirectChatBlocked(bot.uuid, target.id, blocked);
            }
        } catch (toggleError) {
            setBusyTarget(null);
            notify.error((toggleError as Error).message);
            return;
        }
        setBusyTarget(null);
        notify.success(blocked
            ? t('notify.target.blocked', {defaultValue: 'Target blocked'})
            : t('notify.target.unblocked', {defaultValue: 'Target unblocked'}));
        setTargets(prev => prev.map(item => item.id === target.id ? {...item, blocked} : item));
    }, [bot.uuid, t]);

    const handleDelete = useCallback(async (chatID: string) => {
        if (!bot.uuid) return;
        const chat = targets.find((candidate) => candidate.id === chatID && candidate.kind === 'direct_chat');
        if (!chat) return;
        setBusyTarget(chatID);
        try {
            await api.deleteBotDirectChat(bot.uuid, chat.id);
        } catch (deleteError) {
            setBusyTarget(null);
            setDeleteTarget(null);
            notify.error((deleteError as Error).message);
            return;
        }
        setBusyTarget(null);
        setDeleteTarget(null);
        notify.success(t('notify.chat.deleted', {defaultValue: 'Chat deleted'}));
        setTargets(prev => prev.filter(target => target.id !== chatID));
    }, [bot.uuid, targets, t]);

    // The capability probe runner — owns firing notify/confirm against a chat
    // and the per-(chat,capability) results. Lives at the group level so a
    // result persists across re-renders of the chat list.
    const probe = useChatProbe();
    const handleProbe = useCallback((target: NotifyTarget, capability: ChatCapability) => {
        if (!bot.uuid) return;
        void probe.run(bot.uuid, target.id, target.id, target.kind, capability);
    }, [bot.uuid, probe]);

    const handleAllowAndTest = useCallback(async (target: NotifyTarget) => {
        if (!bot.uuid) return;
        setBusyTarget(target.id);
        try {
            if (target.kind === 'group') {
                await api.setBotGroupCapability(bot.uuid, target.id, 'notify', 'allow');
            } else {
                await Promise.all([
                    api.setBotDirectChatPermission(bot.uuid, target.id, 'notify', 'access', 'allow'),
                    api.setBotDirectChatPermission(bot.uuid, target.id, 'notify', 'notify.receive', 'allow'),
                    api.setBotDirectChatPermission(bot.uuid, target.id, 'notify', 'notify.reply', 'allow'),
                ]);
            }
            setTargets(prev => prev.map(item => item.id === target.id ? {...item, can_notify: true, can_reply: target.kind === 'direct_chat'} : item));
            await probe.run(bot.uuid, target.id, target.id, target.kind, 'notify');
        } catch (authorizationError) {
            notify.error((authorizationError as Error).message);
        } finally {
            setBusyTarget(null);
        }
    }, [bot.uuid, probe]);

    // Disabled chats are hidden by default; the footer toggle reveals them
    // (dimmed) so they can be re-enabled or deleted.
    const activeTargets = targets.filter(target => !target.blocked);
    const disabledCount = targets.length - activeTargets.length;
    const visibleTargets = showDisabled ? targets : activeTargets;
    const directCount = activeTargets.filter(target => target.kind === 'direct_chat').length;
    const groupCount = activeTargets.filter(target => target.kind === 'group').length;

    // One sentence answering "can this bot deliver right now?" — the same
    // status-line shape as a Remote Control card, replacing the old chip +
    // "2 direct · 1 groups" + On/Off label.
    const receivers = activeTargets.filter(target => target.can_notify).length;
    const checked = enabled && !loading;
    const status = !enabled
        ? t('notify.group.statusOff', {defaultValue: 'Notify off'})
        : !checked
            ? t('notify.group.statusChecking', {defaultValue: 'Checking targets…'})
            : error
                ? t('notify.group.statusError', {defaultValue: "Couldn't load targets"})
                : receivers > 0
                    ? t('notify.group.statusReceivers', {defaultValue: '{{count}} targets can receive', count: receivers})
                    : activeTargets.length > 0
                        ? t('notify.group.statusNotAllowed', {defaultValue: 'No target allowed yet'})
                        : t('notify.group.statusNoChats', {defaultValue: 'No chats yet'});
    const statusColor = !checked ? 'text.secondary' : (error || receivers === 0) ? 'warning.main' : 'success.main';
    const BrandIcon = PLATFORM_BRAND_ICONS[bot.platform || ''];

    return (
        <Box
            sx={{
                // Off is a quiet state, not a struck-through one: the card
                // drops its paper background and greys its identity, and the
                // body (a stopped bot reaches no chats) is not shown.
                bgcolor: enabled ? 'background.paper' : 'transparent',
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 2,
                transition: 'background-color 0.18s ease-out',
            }}
        >
            {/* Header: platform + bot name, one status line, refresh, switch. */}
            <Box sx={{display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 1, px: 2, py: 1.5}}>
                <Box sx={{display: 'flex', alignItems: 'center', gap: 1.5, minWidth: 0}}>
                    {BrandIcon && <BrandIcon size={24} grayscale={!enabled}/>}
                    <Box sx={{minWidth: 0}}>
                        <Typography noWrap variant="subtitle2" sx={{fontWeight: 600, color: enabled ? 'text.primary' : 'text.secondary'}}>
                            {bot.name || bot.platform}
                        </Typography>
                        <Typography noWrap variant="caption" sx={{display: 'block', color: statusColor}}>
                            {status}
                        </Typography>
                    </Box>
                </Box>
                <Box sx={{display: 'flex', alignItems: 'center', gap: 0.5, flexShrink: 0}}>
                    {enabled && (
                        // Manual refresh: a chat only registers after the bot
                        // actually receives a message on its channel, so the
                        // first view is expected to be stale until re-pulled.
                        <Tooltip title={t('notify.group.refresh', {defaultValue: 'Refresh reachable chats'})}>
                            <span>
                                <IconButton
                                    size="small"
                                    onClick={loadChats}
                                    disabled={loading || isToggling}
                                    aria-label={t('notify.group.refresh', {defaultValue: 'Refresh reachable chats'})}
                                >
                                    {loading ? <CircularProgress size={16}/> : <RefreshIcon fontSize="small"/>}
                                </IconButton>
                            </span>
                        </Tooltip>
                    )}
                    <Tooltip title={enabled
                        ? t('notify.group.disableHint', {defaultValue: 'Disable Notify for this bot'})
                        : t('notify.group.enableHint', {defaultValue: 'Enable Notify. The bot starts automatically if needed.'})}>
                        {/* One operational state: the backend reconciles the
                            capability and the Bot lifecycle as one action. */}
                        <Switch
                            size="small"
                            color="primary"
                            checked={enabled}
                            disabled={isToggling}
                            onChange={(_, checked) => onToggle(bot.uuid!, checked)}
                        />
                    </Tooltip>
                </Box>
            </Box>

            {/* Body: the notify routing graph — API entry → bot channel →
                chat leaves. Same visual language as RemoteControlGraph
                (nodes + arrows + fork border), opposite direction: remote
                control is chat-driven (chat → bot → agent), notify is
                API-driven (API → bot → chat). State is topology: bot off dims
                the whole chain, a disabled chat dims only its leaf. */}
            {enabled && (
            <Box sx={{px: {xs: 1, sm: 2}, pb: 1.5}}>
                {loading ? (
                    <Box sx={{display: 'flex', justifyContent: 'center', py: 2}}>
                        <CircularProgress size={20} />
                    </Box>
                ) : error ? (
                    <Typography variant="body2" sx={{color: 'error.main', py: 1}}>{error}</Typography>
                ) : (
                    // Always the full route, left to right; a narrow card scrolls
                    // this graph sideways instead of stacking it (the flow is
                    // the point). Padding keeps borders and hover rings clear.
                    <Box sx={(theme) => ({...graphRowStyles(theme), gap: theme.spacing(1), py: 1, px: 0.5})}>
                        {/* Source: the authenticated API surface — the concrete
                            path (real uuid, /api/v1 prefix) so the tooltip is a
                            copyable curl target (ux-principles #5/#11). */}
                        <Box sx={{display: 'contents'}}>
                            <NodeContainer>
                                <ApiEntryNode path={`/api/v1/bots/${bot.uuid}/notify`} active={enabled} />
                            </NodeContainer>

                            <ArrowNode direction="forward" />

                            {/* The bot channel the notify API drives. The card
                                header already names the bot, so the node only
                                says which platform delivery goes through. */}
                            <NodeContainer>
                                <ImBotNode imbot={bot} variant="platform" active={enabled} />
                            </NodeContainer>

                            <ArrowNode direction="forward" />
                        </Box>

                        {/* Fork: one branch per reachable chat (mirrors the @tb/@cc
                            fork in RemoteControlGraph). */}
                        {visibleTargets.length === 0 ? (
                            <Typography variant="body2" sx={{color: 'text.disabled', py: 1, minWidth: 220}}>
                                {isPairingRequired(bot)
                                    ? t('notify.group.emptyPairFirst', {defaultValue: 'No chats yet. Pair this bot, then send it a message on {{platform}} — its Chat ID appears here.', platform: bot.platform || 'its platform'})
                                    : t('notify.group.empty', {defaultValue: 'No chats yet. Send any message to this bot on {{platform}} and its Chat ID appears here.', platform: bot.platform || 'its platform'})}
                            </Typography>
                        ) : (
                            <Box
                                sx={{
                                    display: 'flex',
                                    flexDirection: 'column',
                                    gap: 2,
                                    borderLeft: '2px solid',
                                    borderColor: 'divider',
                                    pl: 2,
                                    py: 0.5,
                                    flexShrink: 0,
                                }}
                            >
                                {visibleTargets.map((target) => {
                                    const running = (cap: ChatCapability) => probe.isRunning(target.id, cap);
                                    const anyRunning = running('notify') || running('confirm');
                                    // One lookup per capability per render — reused by the
                                    // button colors and the verdict lines below.
                                    const results = (['notify', 'confirm'] as const)
                                        .map((cap) => ({cap, result: probe.getResult(target.id, cap)}))
                                        .filter((r): r is {cap: ChatCapability; result: ChatProbeResult} => Boolean(r.result));
                                    const resultFor = (cap: ChatCapability) => results.find((r) => r.cap === cap)?.result;
                                    const benchUsable = enabled && !target.blocked;
                                    return (
                                        <Box key={`${target.kind}:${target.id}`} sx={{display: 'flex', alignItems: 'flex-start', gap: 1}}>
                                            {/* The chat leaf. */}
                                            <NodeContainer>
                                                <ChatNode
                                                    chatID={target.external_id}
                                                    targetID={target.id}
                                                    kind={target.kind}
                                                    name={target.name}
                                                    isPaired={target.is_paired}
                                                    active={enabled}
                                                    blocked={target.blocked}
                                                />
                                            </NodeContainer>

                                            {/* Beside the node, vertically centered: one action
                                                row — probe bench left, lifecycle icons pushed
                                                right — with verdicts underneath. One row, two
                                                zones: "use the chat" vs "manage the chat". */}
                                            <Box sx={{display: 'flex', flexDirection: 'column', gap: 0.75, justifyContent: 'center', alignSelf: 'stretch'}}>
                                                <Box sx={{display: 'flex', alignItems: 'center', gap: 0.75, flexWrap: 'nowrap'}}>
                                                    {/* Probe bench in a fixed-width slot, so the
                                                        lifecycle icons line up across rows that
                                                        offer different probes. */}
                                                    <Box sx={{display: 'flex', alignItems: 'center', gap: 0.75, width: 280, flexShrink: 0}}>
                                                    {/* Probe bench — hidden for a disabled chat:
                                                        the backend 404s pushes to it, so the
                                                        buttons would only manufacture failures.
                                                        Gated (not-yet-wired) capabilities are
                                                        skipped, not rendered dead. */}
                                                    {benchUsable && !target.can_notify && (
                                                        <Button
                                                            size="small"
                                                            variant="contained"
                                                            disabled={busyTarget === target.id || anyRunning}
                                                            onClick={() => void handleAllowAndTest(target)}
                                                            sx={{textTransform: 'none'}}
                                                        >
                                                            {busyTarget === target.id
                                                                ? <CircularProgress size={14} color="inherit"/>
                                                                : t('notify.group.allowAndTest', {defaultValue: 'Allow Notify & Test'})}
                                                        </Button>
                                                    )}
                                                    {benchUsable && target.can_notify && (<>
                                                        {CHAT_CAPABILITIES.filter((cap) => !cap.gated && (cap.capability !== 'confirm' || target.can_reply)).map((cap) => {
                                                            const capability = cap.capability as ChatCapability;
                                                            const isRunning = running(capability);
                                                            const result = resultFor(capability);
                                                            const v = result ? verdict(result) : null;
                                                            return (
                                                                <Tooltip key={cap.capability} title={cap.hint}>
                                                                    <span>
                                                                        <Button
                                                                            size="small"
                                                                            variant="outlined"
                                                                            color={v ? (v.severity === 'info' ? 'primary' : v.severity) : 'primary'}
                                                                            disabled={isRunning || anyRunning}
                                                                            onClick={() => handleProbe(target, capability)}
                                                                            startIcon={isRunning ? <CircularProgress size={14} color="inherit" /> : cap.icon}
                                                                            sx={{textTransform: 'none'}}
                                                                        >
                                                                            {cap.label}
                                                                        </Button>
                                                                    </span>
                                                                </Tooltip>
                                                            );
                                                        })}
                                                        {/* Custom → free-form editor. Same outlined
                                                            variant as the probe buttons — it sits in
                                                            the same bench and acts on the same chat. */}
                                                        <Tooltip title={t('notify.group.customHint', {defaultValue: 'Compose a custom message (free-form)'})}>
                                                            <Button
                                                                size="small"
                                                                variant="outlined"
                                                                color="primary"
                                                                startIcon={<CustomIcon fontSize="small" />}
                                                                onClick={() => openTest(target.id)}
                                                                sx={{textTransform: 'none'}}
                                                            >
                                                                {t('notify.group.custom', {defaultValue: 'Custom'})}
                                                            </Button>
                                                        </Tooltip>
                                                    </>)}

                                                    </Box>

                                                    {/* Lifecycle zone: copy · disable · delete. */}
                                                    <Tooltip title={t('notify.group.copyChatId', {defaultValue: 'Copy internal target UUID'})}>
                                                        <IconButton size="small" onClick={() => handleCopy(target.id)}>
                                                            <CopyIcon fontSize="small" />
                                                        </IconButton>
                                                    </Tooltip>
                                                    <Tooltip title={target.blocked
                                                        ? t('notify.group.enableChat', {defaultValue: 'Enable — accept its messages again'})
                                                        : t('notify.group.disableChat', {defaultValue: 'Disable — silently drop its messages'})}>
                                                        <span>
                                                            <IconButton
                                                                size="small"
                                                                color={target.blocked ? 'default' : 'warning'}
                                                                disabled={busyTarget === target.id}
                                                                onClick={() => handleToggleDisabled(target)}
                                                                aria-label={target.blocked
                                                                    ? t('notify.group.enableChat', {defaultValue: 'Enable chat'})
                                                                    : t('notify.group.disableChat', {defaultValue: 'Disable chat'})}
                                                            >
                                                                <BlockIcon fontSize="small" />
                                                            </IconButton>
                                                        </span>
                                                    </Tooltip>
                                                    {target.kind === 'direct_chat' && (
                                                        <Tooltip title={t('notify.group.deleteChat', {defaultValue: 'Delete this Direct Chat record'})}>
                                                            <span>
                                                                <IconButton
                                                                    size="small"
                                                                    color="error"
                                                                    disabled={busyTarget === target.id}
                                                                    onClick={() => setDeleteTarget(target.id)}
                                                                    aria-label={t('notify.group.deleteChat', {defaultValue: 'Delete Direct Chat'})}
                                                                >
                                                                    <DeleteIcon fontSize="small" />
                                                                </IconButton>
                                                            </span>
                                                        </Tooltip>
                                                    )}
                                                </Box>

                                                {/* Inline probe results (one per active capability that has run). */}
                                                {results.length > 0 && (
                                                    <Stack spacing={0.75}>
                                                        {results.map(({cap, result}) => (
                                                            <ProbeResultLine key={cap} result={result} onDismiss={() => probe.clear(target.id, cap)} />
                                                        ))}
                                                    </Stack>
                                                )}
                                            </Box>
                                        </Box>
                                    );
                                })}
                            </Box>
                        )}
                    </Box>
                )}
                {!loading && !error && enabled && disabledCount > 0 && (
                    <Box sx={{mt: 1, textAlign: 'right'}}>
                        <Link
                            component="button"
                            variant="caption"
                            underline="hover"
                            onClick={() => setShowDisabled(v => !v)}
                            sx={{color: 'text.secondary'}}
                        >
                            {showDisabled
                                ? t('notify.group.hideDisabled', {defaultValue: 'Hide disabled'})
                                : t('notify.group.showDisabled', {defaultValue: 'Show disabled ({{count}})', count: disabledCount})}
                        </Link>
                    </Box>
                )}
            </Box>
            )}

            {/* Delete confirm — hard delete is destructive-but-recoverable
                (natural re-register), so the dialog states exactly what goes
                and points to Disable as the blocking alternative. */}
            <ConfirmDialog
                open={deleteTarget !== null}
                title={t('notify.group.deleteChatTitle', {defaultValue: 'Delete this chat?'})}
                description={
                    <>
                        <Typography variant="body2" sx={{fontFamily: fontMono, mb: 1}}>{deleteTarget}</Typography>
                        <Typography variant="body2">
                            {t('notify.group.deleteChatBody', {
                                defaultValue: 'Its pairing, whitelist, and project binding are removed. If it messages the bot again it re-registers as a brand-new chat (re-pairing required when pairing is enforced). Session history is untouched. To block it instead, use Disable.',
                            })}
                        </Typography>
                    </>
                }
                confirmLabel={t('common.delete', {defaultValue: 'Delete'})}
                confirmColor="error"
                loading={busyTarget !== null && busyTarget === deleteTarget}
                onClose={() => setDeleteTarget(null)}
                onConfirm={() => deleteTarget && handleDelete(deleteTarget)}
            />

            <NotifyTestDialog
                open={testTargetID !== null}
                botUUID={bot.uuid!}
                botName={bot.name || bot.platform}
                targets={activeTargets.filter(target => target.can_notify)}
                initialTargetID={testTargetID ?? undefined}
                onClose={closeTest}
            />
        </Box>
    );
};

export default BotNotifyGroup;
