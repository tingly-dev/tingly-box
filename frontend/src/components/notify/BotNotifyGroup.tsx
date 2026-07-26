import {ContentCopy as CopyIcon, Edit as CustomIcon, Close as CloseIcon, Code as CodeIcon, Refresh as RefreshIcon} from '@/components/icons';
import {api} from '@/services/api';
import {notify} from '@/utils/notify';
import {isPairingRequired} from '@/types/bot';
import type {BotChat, BotSettings} from '@/types/bot';
import {fontMono} from '@/theme/fonts';
import NotifyTestDialog from '@/components/notify/NotifyTestDialog';
import {CHAT_CAPABILITIES} from '@/components/notify/chatCapabilities';
import useChatProbe, {type ChatCapability, type ChatProbeResult} from '@/components/notify/useChatProbe';
import {
    Alert,
    Box,
    Button,
    Chip,
    CircularProgress,
    Collapse,
    IconButton,
    Stack,
    Switch,
    Tooltip,
    Typography,
} from '@mui/material';
import {useCallback, useEffect, useState} from 'react';
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
// empty list with running:false — we skip the round trip entirely and render
// the disabled body instead.
export interface BotNotifyGroupProps {
    bot: BotSettings;
    onToggle: (uuid: string) => void;
    isToggling?: boolean;
}

// Em-dash placeholder for empty status/project/updated meta — shared styling.
const Dash: React.FC = () => (
    <Typography variant="body2" sx={{color: 'text.secondary'}}>—</Typography>
);

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
                <Box sx={{mt: 1, p: 1, bgcolor: 'action.hover', borderRadius: 1, fontFamily: fontMono, fontSize: '0.75rem', whiteSpace: 'pre-wrap', wordBreak: 'break-all'}}>
                    {JSON.stringify(result.raw ?? result, null, 2)}
                </Box>
            </Collapse>
        </Alert>
    );
};



const BotNotifyGroup: React.FC<BotNotifyGroupProps> = ({bot, onToggle, isToggling}) => {
    const {t} = useTranslation();
    const enabled = bot.enabled ?? true;

    const [chats, setChats] = useState<BotChat[]>([]);
    const [loading, setLoading] = useState(false);
    const [error, setError] = useState<string | null>(null);
    const [testChatID, setTestChatID] = useState<string | null>(null);

    const loadChats = useCallback(async () => {
        if (!bot.uuid) return;
        setLoading(true);
        setError(null);
        const result = await api.listBotChats(bot.uuid);
        setLoading(false);
        if (result.error) {
            setError(result.error);
        } else {
            setChats(result.chats ?? []);
        }
    }, [bot.uuid]);

    // Eager-load only when the bot is enabled (a stopped bot has no reachable
    // chats). Re-fetch on enable transitions so toggling on surfaces fresh chats.
    useEffect(() => {
        if (enabled) loadChats();
        else {
            setChats([]);
            setError(null);
        }
    }, [enabled, loadChats]);

    const handleCopy = useCallback(async (chatID: string) => {
        try {
            await navigator.clipboard.writeText(chatID);
            notify.success(t('notify.chat.copied', {defaultValue: 'Chat ID copied'}));
        } catch {
            notify.error(t('notify.chat.copyFailed', {defaultValue: 'Copy failed — check clipboard permissions'}));
        }
    }, [t]);

    const openTest = useCallback((chatID: string) => setTestChatID(chatID), []);
    const closeTest = useCallback(() => setTestChatID(null), []);

    // The capability probe runner — owns firing notify/confirm against a chat
    // and the per-(chat,capability) results. Lives at the group level so a
    // result persists across re-renders of the chat list.
    const probe = useChatProbe();
    const handleProbe = useCallback((chatID: string, capability: ChatCapability) => {
        if (!bot.uuid) return;
        void probe.run(bot.uuid, chatID, capability);
    }, [bot.uuid, probe]);

    return (
        <Box
            sx={{
                border: '1px solid',
                borderColor: 'divider',
                borderRadius: 1.5,
                overflow: 'hidden',
            }}
        >
            {/* Header: name + platform + enabled switch (the on/off for driving
                this bot) + chat count. The switch is the bot's existing enabled
                flag — surfaced here because "can I use this bot to notify?" is
                exactly the question this page answers. */}
            <Box
                sx={{
                    display: 'flex',
                    alignItems: 'center',
                    gap: 1.5,
                    px: 2,
                    py: 1.25,
                    bgcolor: 'action.hover',
                    flexWrap: 'wrap',
                }}
            >
                {/* Fixed-width name column so every group's name chip aligns
                    across rows — name length varies, but the column shouldn't. */}
                <Tooltip title={bot.name || bot.platform}>
                    <Typography noWrap variant="body2" sx={{fontWeight: 600, flexShrink: 0, width: {xs: 96, sm: 150}}}>
                        {bot.name || bot.platform}
                    </Typography>
                </Tooltip>
                <Chip label={bot.platform} size="small" />
                <Box sx={{flexGrow: 1}} />
                {enabled && (
                    <Typography variant="body2" sx={{color: 'text.secondary'}}>
                        {chats.length > 0
                            ? t('notify.group.chatCount', {defaultValue: '{{count}} reachable chat(s)', count: chats.length})
                            : t('notify.group.noChats', {defaultValue: 'No reachable chats'})}
                    </Typography>
                )}
                {enabled && (
                    // Manual refresh: a chat only registers after the bot
                    // actually receives a message on its channel, so the first
                    // view is expected to be stale until the operator re-pulls.
                    <Tooltip title={t('notify.group.refresh', {defaultValue: 'Refresh reachable chats'})}>
                        <IconButton
                            size="small"
                            onClick={loadChats}
                            disabled={loading || isToggling}
                            aria-label={t('notify.group.refresh', {defaultValue: 'Refresh reachable chats'})}
                        >
                            {loading ? <CircularProgress size={16}/> : <RefreshIcon fontSize="small"/>}
                        </IconButton>
                    </Tooltip>
                )}
                <Tooltip title={enabled
                    ? t('notify.group.disableHint', {defaultValue: 'Disable this bot'})
                    : t('notify.group.enableHint', {defaultValue: 'Enable this bot to send notifications'})}>
                    {/* The switch is wired to the same toggle the Bots table uses
                        (POST /imbot-settings/:uuid/toggle) — it starts/stops the
                        bot's channel, which is what governs whether notify can
                        reach it. A disabled-bot row shows why chats are absent. */}
                    <Stack
                        direction="row"
                        spacing={0.75}
                        sx={{alignItems: 'center', cursor: isToggling ? 'wait' : 'pointer'}}
                    >
                        <Switch
                            size="small"
                            color="success"
                            checked={enabled}
                            disabled={isToggling}
                            onChange={() => onToggle(bot.uuid!)}
                        />
                        {isToggling ? (
                            <CircularProgress size={14} />
                        ) : (
                            <Typography variant="body2" sx={{color: enabled ? 'success.main' : 'text.secondary', fontWeight: 600}}>
                                {enabled ? t('common.on', {defaultValue: 'On'}) : t('common.off', {defaultValue: 'Off'})}
                            </Typography>
                        )}
                    </Stack>
                </Tooltip>
            </Box>

            {/* Body: the reachable chats, always expanded (no extra click). */}
            <Box sx={{px: {xs: 1, sm: 2}, py: 1.5}}>
                {!enabled ? (
                    <Typography variant="body2" sx={{color: 'text.disabled', py: 1}}>
                        {t('notify.group.disabledBody', {defaultValue: 'Bot is off — enable it to see and send to its reachable chats.'})}
                    </Typography>
                ) : loading ? (
                    <Box sx={{display: 'flex', justifyContent: 'center', py: 2}}>
                        <CircularProgress size={20} />
                    </Box>
                ) : error ? (
                    <Typography variant="body2" sx={{color: 'error.main', py: 1}}>{error}</Typography>
                ) : chats.length === 0 ? (
                    <Typography variant="body2" sx={{color: 'text.disabled', py: 1}}>
                        {isPairingRequired(bot)
                            ? t('notify.group.emptyPairFirst', {defaultValue: 'No chats yet. Pair this bot, then send it a message on {{platform}} — its Chat ID appears here.', platform: bot.platform || 'its platform'})
                            : t('notify.group.empty', {defaultValue: 'No chats yet. Send any message to this bot on {{platform}} and its Chat ID appears here.', platform: bot.platform || 'its platform'})}
                    </Typography>
                ) : (
                    <Stack spacing={1}>
                        {chats.map((chat) => {
                            const running = (cap: ChatCapability) => probe.isRunning(chat.chat_id, cap);
                            const anyRunning = running('notify') || running('confirm');
                            return (
                                <Box
                                    key={chat.chat_id}
                                    sx={{
                                        border: '1px solid',
                                        borderColor: 'divider',
                                        borderRadius: 1,
                                        p: 1.25,
                                    }}
                                >
                                    {/* Chat identity + meta row */}
                                    <Box sx={{display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap'}}>
                                        <Typography variant="body2" component="span" sx={{fontFamily: fontMono, color: 'text.primary', fontWeight: 600}}>
                                            {chat.chat_id}
                                        </Typography>
                                        {chat.is_paired ? (
                                            <Chip label={t('notify.group.paired', {defaultValue: 'paired'})} size="small" color="success" variant="outlined" />
                                        ) : <Dash/>}
                                        {chat.project_path && (
                                            <Tooltip title={chat.project_path}>
                                                <Typography variant="caption" component="span" noWrap sx={{color: 'text.secondary', maxWidth: 220}}>
                                                    {chat.project_path}
                                                </Typography>
                                            </Tooltip>
                                        )}
                                        {chat.updated_at && (
                                            <Typography variant="caption" sx={{color: 'text.disabled'}}>
                                                {new Date(chat.updated_at).toLocaleString()}
                                            </Typography>
                                        )}
                                        <Box sx={{flexGrow: 1}} />
                                        <Tooltip title={t('notify.group.copyChatId', {defaultValue: 'Copy Chat ID'})}>
                                            <IconButton size="small" onClick={() => handleCopy(chat.chat_id)}>
                                                <CopyIcon fontSize="small" />
                                            </IconButton>
                                        </Tooltip>
                                    </Box>

                                    {/* Capability probe controls + inline verdict.
                                        Mirrors the probe feature: one-click trigger
                                        per capability, result renders inline. */}
                                    <Box sx={{display: 'flex', alignItems: 'center', gap: 0.75, flexWrap: 'wrap', mt: 1}}>
                                        {CHAT_CAPABILITIES.map((cap) => {
                                            // `gated` is chatCapabilities' own answer to "is this
                                            // capability wired end-to-end yet?" — read it rather
                                            // than re-listing the runnable ones here, so the two
                                            // can't drift apart.
                                            const active = !cap.gated;
                                            const isRunning = active && running(cap.capability as ChatCapability);
                                            const result = active ? probe.getResult(chat.chat_id, cap.capability as ChatCapability) : undefined;
                                            const v = result ? verdict(result) : null;
                                            return (
                                                <Tooltip key={cap.capability} title={cap.hint}>
                                                    <span>
                                                        <Button
                                                            size="small"
                                                            variant="outlined"
                                                            color={v?.severity === 'success' ? 'success' : v?.severity === 'error' ? 'error' : v?.severity === 'warning' ? 'warning' : 'primary'}
                                                            disabled={!active || isRunning || anyRunning}
                                                            onClick={() => active && handleProbe(chat.chat_id, cap.capability as ChatCapability)}
                                                            startIcon={isRunning ? <CircularProgress size={14} color="inherit" /> : cap.icon}
                                                            sx={{textTransform: 'none'}}
                                                        >
                                                            {cap.label}
                                                        </Button>
                                                    </span>
                                                </Tooltip>
                                            );
                                        })}
                                        {/* Custom → free-form editor (the escape hatch). */}
                                        <Tooltip title={t('notify.group.customHint', {defaultValue: 'Compose a custom message (free-form)'})}>
                                            <Button
                                                size="small"
                                                variant="text"
                                                color="primary"
                                                startIcon={<CustomIcon fontSize="small" />}
                                                onClick={() => openTest(chat.chat_id)}
                                                sx={{textTransform: 'none'}}
                                            >
                                                {t('notify.group.custom', {defaultValue: 'Custom'})}
                                            </Button>
                                        </Tooltip>
                                    </Box>

                                    {/* Inline probe results (one per active capability that has run). */}
                                    {(probe.getResult(chat.chat_id, 'notify') || probe.getResult(chat.chat_id, 'confirm')) && (
                                        <Stack spacing={0.75} sx={{mt: 1}}>
                                            {(['notify', 'confirm'] as const).map((cap) => {
                                                const r = probe.getResult(chat.chat_id, cap);
                                                if (!r) return null;
                                                return <ProbeResultLine key={cap} result={r} onDismiss={() => probe.clear(chat.chat_id, cap)} />;
                                            })}
                                        </Stack>
                                    )}
                                </Box>
                            );
                        })}
                    </Stack>
                )}
            </Box>

            <NotifyTestDialog
                open={testChatID !== null}
                botUUID={bot.uuid!}
                botName={bot.name || bot.platform}
                chats={chats}
                initialChatID={testChatID ?? undefined}
                onClose={closeTest}
            />
        </Box>
    );
};

export default BotNotifyGroup;
