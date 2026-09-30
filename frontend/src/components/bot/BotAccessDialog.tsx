import { Security } from '@/components/icons';
import { api } from '@/services/api';
import type {BotCapability, BotGroupDetail, BotSettings, DirectChatDetail, CapabilityName} from '@/types/bot';
import { chatCanControl, chatPermissionAllowed, isPairingRequired, remoteChatState } from '@/types/bot';
import {
    Alert, Box, Button, Chip, CircularProgress, Dialog, DialogActions, DialogContent,
    DialogTitle, Divider, FormControlLabel, Stack, Switch, TextField, ToggleButton, ToggleButtonGroup, Tooltip, Typography,
} from '@mui/material';
import type {ReactNode} from 'react';
import {useCallback, useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';
import PairingCodePanel from './PairingCodePanel';
import type {RemoteAccess} from './useRemoteAccess';

interface Props {
    open: boolean;
    bot: BotSettings | null;
    onClose: () => void;
    onChanged?: () => void;
    /**
     * 'all' (Bots page): the bot resource's whole access surface — UUID,
     * capability switches, Notify and Remote per chat.
     * 'remote_control' (Remote page): only "who can control this bot". The
     * capability switch already lives on the Remote card, and Notify belongs
     * to IM Notify, so neither is repeated here.
     */
    scope?: 'all' | 'remote_control';
    /**
     * scope='remote_control' only: the card's own useRemoteAccess. The dialog
     * renders and reloads that copy instead of fetching the same chats and
     * groups again, so the card's summary and the dialog can't drift.
     */
    remoteAccess?: RemoteAccess;
}

type ChatControl = 'control' | 'none' | 'blocked';

const rowSx = {p: 1.5, border: 1, borderColor: 'divider', borderRadius: 1.5} as const;

const rowHeader = (title: ReactNode, id: string) => (
    <Box sx={{minWidth: 0}}>
        <Typography sx={{fontWeight: 600}}>{title}</Typography>
        <Typography variant="caption" sx={{fontFamily: 'monospace', color: 'text.primary', overflowWrap: 'anywhere'}}>{id}</Typography>
    </Box>
);

const section = (title: string, hint: string, body: ReactNode) => (
    <Box>
        <Typography variant="h6">{title}</Typography>
        <Typography variant="body2" color="text.secondary" sx={{mb: 1}}>{hint}</Typography>
        {body}
    </Box>
);

// Remote rows are always written as one atomic batch: a partial failure must
// never leave start=allow with approve=deny, which silently breaks permission
// replies. Privileged stays denied from the UI.
const remoteRows = (allow: boolean) => {
    const effect = allow ? 'allow' as const : 'deny' as const;
    return [
        {capability: 'remote_control', action: 'access', effect},
        {capability: 'remote_control', action: 'remote_control.start', effect},
        {capability: 'remote_control', action: 'remote_control.approve', effect},
        {capability: 'remote_control', action: 'remote_control.privileged', effect: 'deny' as const},
    ];
};

const BotAccessDialog = ({open, bot, onClose, onChanged, scope = 'all', remoteAccess}: Props) => {
    const {t} = useTranslation();
    const remoteOnly = scope === 'remote_control';
    const shared = remoteOnly ? remoteAccess : undefined;
    const [capabilities, setCapabilities] = useState<BotCapability[]>([]);
    const [ownChats, setOwnChats] = useState<DirectChatDetail[]>([]);
    const [ownGroups, setOwnGroups] = useState<BotGroupDetail[]>([]);
    const [ownLoading, setOwnLoading] = useState(false);
    const [saving, setSaving] = useState(false);
    const [error, setError] = useState('');
    const [actorGroup, setActorGroup] = useState<BotGroupDetail | null>(null);
    const [externalActorID, setExternalActorID] = useState('');
    const [actorName, setActorName] = useState('');

    const botUuid = bot?.uuid;
    const loadOwn = useCallback(async () => {
        if (!botUuid) return;
        setOwnLoading(true);
        setError('');
        try {
            const [capabilityData, chatData, groupData] = await Promise.all([
                api.listBotCapabilities(botUuid), api.listBotDirectChats(botUuid), api.listBotGroups(botUuid),
            ]);
            const details = await Promise.all((groupData.groups || []).map((group) => api.getBotGroup(botUuid, group.id)));
            setCapabilities(capabilityData.capabilities || []);
            setOwnChats(chatData.chats || []);
            setOwnGroups(details);
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setOwnLoading(false);
        }
    }, [botUuid]);
    const load = shared?.reload ?? loadOwn;
    const chats = shared ? shared.chats : ownChats;
    const groups = shared ? shared.groups : ownGroups;
    const loading = shared ? shared.loading : ownLoading;
    const shownError = error || shared?.error || '';

    useEffect(() => { if (open) void load(); }, [open, load]);

    const mutate = async (action: () => Promise<unknown>) => {
        setSaving(true);
        setError('');
        try {
            await action();
            await load();
            onChanged?.();
        } catch (e) {
            setError((e as Error).message);
        } finally {
            setSaving(false);
        }
    };
    const capabilityOn = (name: CapabilityName) => capabilities.find((item) => item.capability === name)?.enabled === true;

    const setPreset = (chat: DirectChatDetail, preset: 'full' | 'notify') => mutate(async () => {
        const notifyRows = ['access', 'notify.receive', 'notify.reply'].map((action) =>
            ({capability: 'notify', action, effect: 'allow' as const}));
        await api.setBotDirectChatPermissions(bot!.uuid!, chat.chat.id, [...notifyRows, ...remoteRows(preset === 'full')]);
    });
    const setChatControl = (chat: DirectChatDetail, next: ChatControl) => mutate(async () => {
        if (next === 'blocked') {
            await api.setBotDirectChatBlocked(bot!.uuid!, chat.chat.id, true);
            return;
        }
        // Rows first, unblock second: if the rows write fails, a chat being
        // cut off must stay blocked rather than come back with its old allows.
        await api.setBotDirectChatPermissions(bot!.uuid!, chat.chat.id, remoteRows(next === 'control'));
        if (chat.chat.blocked) await api.setBotDirectChatBlocked(bot!.uuid!, chat.chat.id, false);
    });

    const chatHeader = (chat: DirectChatDetail) => rowHeader(
        chat.chat.peer_actor_id
            ? t('botAccess.pairedPerson', {defaultValue: 'Paired person'})
            : t('botAccess.unpairedChat', {defaultValue: 'Unpaired chat'}),
        chat.chat.external_chat_id,
    );

    const remoteChatRow = (chat: DirectChatDetail) => {
        const state = remoteChatState(chat);
        const value: ChatControl = chat.chat.blocked ? 'blocked' : chatCanControl(chat) ? 'control' : 'none';
        const canPair = Boolean(chat.chat.peer_actor_id);
        return (
            <Box key={chat.chat.id} sx={rowSx}>
                <Stack direction={{xs: 'column', sm: 'row'}} sx={{justifyContent: 'space-between', alignItems: {sm: 'center'}, gap: 1}}>
                    {chatHeader(chat)}
                    <ToggleButtonGroup
                        exclusive
                        size="small"
                        color="primary"
                        sx={{
                            width: {xs: '100%', sm: 'auto'},
                            '& .MuiToggleButton-root': {flex: {xs: 1, sm: 'none'}, whiteSpace: 'nowrap', textTransform: 'none', px: {xs: 0.5, sm: 1.5}},
                        }}
                        value={value}
                        disabled={saving}
                        // exclusive: re-clicking the selected button yields null
                        onChange={(_, next: ChatControl | null) => { if (next) void setChatControl(chat, next); }}
                    >
                        <ToggleButton value="control" disabled={saving || !canPair}>
                            {t('botAccess.canControl', {defaultValue: 'Can control'})}
                        </ToggleButton>
                        <ToggleButton value="none">{t('botAccess.noAccess', {defaultValue: 'No access'})}</ToggleButton>
                        <ToggleButton value="blocked">{t('botAccess.blocked', {defaultValue: 'Blocked'})}</ToggleButton>
                    </ToggleButtonGroup>
                </Stack>
                {!canPair && (
                    <Typography variant="caption" sx={{display: 'block', mt: 0.5, color: 'text.secondary'}}>
                        {t('botAccess.unpairedHint', {defaultValue: 'Pair first — an unpaired chat can\'t be given control.'})}
                    </Typography>
                )}
                {value === 'none' && (state !== 'off' || chatPermissionAllowed(chat, 'remote_control', 'access')) && (
                    <Typography variant="caption" sx={{display: 'block', mt: 0.5, color: 'warning.main'}}>
                        {t('botAccess.partial', {defaultValue: 'Partly allowed, so permission prompts can\'t be answered. Choose Can control to repair.'})}
                    </Typography>
                )}
            </Box>
        );
    };

    const fullChatRow = (chat: DirectChatDetail) => {
        const state = remoteChatState(chat);
        return (
            <Box key={chat.chat.id} sx={rowSx}>
                <Stack direction={{xs: 'column', sm: 'row'}} sx={{justifyContent: 'space-between', gap: 1}}>
                    {chatHeader(chat)}
                    <Stack direction="row" spacing={1} sx={{flexWrap: 'wrap', alignItems: 'center'}}>
                        {state === 'on' && <Chip size="small" color="primary" label={t('botAccess.chipRemote', {defaultValue: 'Remote Control'})}/>}
                        {state === 'off' && <Chip size="small" label={t('botAccess.chipNoRemote', {defaultValue: 'No Remote Control'})}/>}
                        {(state === 'startDenied' || state === 'approveDenied') && (
                            <Tooltip title={t('botAccess.chipRemoteBrokenHint', {defaultValue: 'start launches runs; approve answers permission/question prompts. Re-apply Full access to repair.'})}>
                                <Chip size="small" color="warning" label={state === 'approveDenied'
                                    ? t('botAccess.chipApproveDenied', {defaultValue: 'Remote Control broken: approve denied'})
                                    : t('botAccess.chipStartDenied', {defaultValue: 'Remote Control broken: start denied'})}/>
                            </Tooltip>
                        )}
                        <Chip size="small" label={chatPermissionAllowed(chat, 'notify', 'notify.receive')
                            ? t('botAccess.chipNotify', {defaultValue: 'Notify'})
                            : t('botAccess.chipNoNotify', {defaultValue: 'No Notify'})}/>
                        <FormControlLabel
                            sx={{m: 0}}
                            control={<Switch size="small" checked={chat.chat.blocked} disabled={saving}
                                onChange={(_, blocked) => void mutate(() => api.setBotDirectChatBlocked(bot!.uuid!, chat.chat.id, blocked))}/>}
                            label={t('botAccess.blocked', {defaultValue: 'Blocked'})}
                        />
                    </Stack>
                </Stack>
                <Stack direction="row" spacing={1} sx={{mt: 1}}>
                    <Button size="small" disabled={saving || !chat.chat.peer_actor_id} onClick={() => void setPreset(chat, 'full')}>
                        {t('botAccess.fullAccess', {defaultValue: 'Full access'})}
                    </Button>
                    <Button size="small" disabled={saving} onClick={() => void setPreset(chat, 'notify')}>
                        {t('botAccess.notifyOnly', {defaultValue: 'Notify only'})}
                    </Button>
                </Stack>
            </Box>
        );
    };

    const groupRow = ({group, capabilities: groupCaps, actors}: BotGroupDetail) => {
        const remoteAllowed = groupCaps.remote_control === 'allow';
        const nobody = remoteAllowed && actors.length === 0;
        const capSwitch = (name: CapabilityName, label: string) => (
            <FormControlLabel
                control={<Switch size="small" checked={groupCaps[name] === 'allow'} disabled={saving}
                    onChange={(_, enabled) => void mutate(() => api.setBotGroupCapability(bot!.uuid!, group.id, name, enabled ? 'allow' : 'deny'))}/>}
                label={label}
            />
        );
        return (
            <Box key={group.id} sx={rowSx}>
                <Stack direction={{xs: 'column', sm: 'row'}} sx={{justifyContent: 'space-between', gap: 1}}>
                    {rowHeader(group.name || t('botAccess.group', {defaultValue: 'Group'}), group.external_group_id)}
                    <Stack direction="row">
                        {!remoteOnly && capSwitch('notify', t('botAccess.notify', {defaultValue: 'Notify'}))}
                        {capSwitch('remote_control', t('botAccess.remoteControl', {defaultValue: 'Remote Control'}))}
                    </Stack>
                </Stack>
                {(remoteAllowed || !remoteOnly) && (
                    <Stack direction="row" sx={{alignItems: 'center', gap: 1, mt: 0.5, flexWrap: 'wrap'}}>
                        <Typography variant="body2" color={nobody ? 'warning.main' : 'text.secondary'}>
                            {t('botAccess.actors', {defaultValue: '{{count}} authorized actors', count: actors.length})}
                            {nobody && ` — ${t('botAccess.nobodyInGroup', {defaultValue: 'nobody here can control this Bot yet.'})}`}
                        </Typography>
                        {actors.map(({actor}) => <Chip key={actor.id} size="small" label={actor.display_name || actor.external_actor_id}/>)}
                        <Button size="small" onClick={() => {
                            setActorGroup({group, capabilities: groupCaps, actors});
                            setExternalActorID('');
                            setActorName('');
                        }}>
                            {t('botAccess.addActor', {defaultValue: 'Add actor'})}
                        </Button>
                    </Stack>
                )}
            </Box>
        );
    };

    const name = bot?.name || bot?.platform || '';

    return <Dialog open={open} onClose={onClose} fullWidth maxWidth="md">
        <DialogTitle sx={{display: 'flex', alignItems: 'center', gap: 1}}>
            <Security color="primary"/>
            {remoteOnly
                ? t('botAccess.titleRemote', {defaultValue: 'Who can control {{name}}', name})
                : t('botAccess.titleAll', {defaultValue: '{{name}} access', name})}
        </DialogTitle>
        <DialogContent dividers>
            <Stack spacing={3}>
                {remoteOnly ? (
                    // The next action for adding someone, handed over as the
                    // literal command rather than a hidden code.
                    <Box>
                        <Typography variant="body2" color="text.secondary">
                            {isPairingRequired(bot)
                                ? t('botAccess.addPairing', {defaultValue: 'To add someone, they send this to the bot in a direct message:'})
                                : t('botAccess.addMessage', {defaultValue: 'To add someone, they message the bot in a direct chat. The chat then shows up below.'})}
                        </Typography>
                        {bot && <PairingCodePanel bot={bot} revealByDefault/>}
                    </Box>
                ) : (
                    <Box>
                        <Typography variant="body2" color="text.secondary">{t('botAccess.botUuid', {defaultValue: 'Bot UUID'})}</Typography>
                        <Typography component="code" sx={{fontFamily: 'monospace', wordBreak: 'break-all'}}>{bot?.uuid}</Typography>
                        {bot && <PairingCodePanel bot={bot}/>}
                    </Box>
                )}
                {shownError && <Alert severity="error">{shownError}</Alert>}
                {loading ? <Box sx={{display: 'flex', justifyContent: 'center', py: 6}}><CircularProgress/></Box> : <>
                    {!remoteOnly && <>
                        {section(
                            t('botAccess.capabilities', {defaultValue: 'Capabilities'}),
                            t('botAccess.capabilitiesHint', {defaultValue: 'What this Bot can provide. Turning off the last capability stops the connection without deleting its configuration.'}),
                            <Stack direction={{xs: 'column', sm: 'row'}} spacing={3}>
                                {(['remote_control', 'notify'] as CapabilityName[]).map((cap) => (
                                    <FormControlLabel
                                        key={cap}
                                        control={<Switch checked={capabilityOn(cap)} disabled={saving}
                                            onChange={(_, enabled) => void mutate(() => api.setBotCapability(bot!.uuid!, cap, enabled))}/>}
                                        label={cap === 'remote_control'
                                            ? t('botAccess.remoteControl', {defaultValue: 'Remote Control'})
                                            : t('botAccess.notify', {defaultValue: 'Notify'})}
                                    />
                                ))}
                            </Stack>,
                        )}
                        <Divider/>
                    </>}
                    {section(
                        t('botAccess.directChats', {defaultValue: 'Direct Chats'}),
                        remoteOnly
                            ? t('botAccess.directChatsRemoteHint', {defaultValue: 'People who reached the bot in a direct message, and whether each can control it.'})
                            : t('botAccess.directChatsHint', {defaultValue: 'Who is paired, what they can do, and the concrete platform chat ID.'}),
                        chats.length === 0
                            ? <Alert severity="info">{t('botAccess.noDirectChats', {defaultValue: 'No Direct Chats yet.'})}</Alert>
                            : <Stack spacing={1.5}>{chats.map((chat) => remoteOnly ? remoteChatRow(chat) : fullChatRow(chat))}</Stack>,
                    )}
                    <Divider/>
                    {section(
                        t('botAccess.groups', {defaultValue: 'Groups'}),
                        remoteOnly
                            ? t('botAccess.groupsRemoteHint', {defaultValue: 'Allow Remote Control in a group, then add the people in it who may use it.'})
                            : t('botAccess.groupsHint', {defaultValue: 'Group capability access and authorized Actors are separate controls.'}),
                        groups.length === 0
                            ? <Alert severity="info">{t('botAccess.noGroups', {defaultValue: 'No Groups observed yet. Add the Bot to a group and send it a message; new groups start with no access.'})}</Alert>
                            : <Stack spacing={1.5}>{groups.map(groupRow)}</Stack>,
                    )}
                </>}
            </Stack>
        </DialogContent>
        <DialogActions><Button onClick={onClose}>{t('common.close', {defaultValue: 'Close'})}</Button></DialogActions>
        <Dialog open={Boolean(actorGroup)} onClose={() => setActorGroup(null)} fullWidth maxWidth="xs">
            <DialogTitle>{t('botAccess.addActorTitle', {defaultValue: 'Add authorized actor'})}</DialogTitle>
            <DialogContent>
                <Stack spacing={2} sx={{pt: 1}}>
                    <Alert severity="info">{t('botAccess.addActorInfo', {defaultValue: 'This grants Start and Approve in this Group. Privileged access stays denied.'})}</Alert>
                    <TextField
                        autoFocus
                        label={t('botAccess.actorId', {defaultValue: 'Platform actor ID'})}
                        value={externalActorID}
                        onChange={(event) => setExternalActorID(event.target.value)}
                        helperText={t('botAccess.actorIdHint', {defaultValue: 'Use the concrete user ID reported by the IM platform.'})}
                    />
                    <TextField
                        label={t('botAccess.displayName', {defaultValue: 'Display name (optional)'})}
                        value={actorName}
                        onChange={(event) => setActorName(event.target.value)}
                    />
                </Stack>
            </DialogContent>
            <DialogActions>
                <Button onClick={() => setActorGroup(null)}>{t('common.cancel', {defaultValue: 'Cancel'})}</Button>
                <Button
                    variant="contained"
                    disabled={!externalActorID.trim() || saving}
                    onClick={() => void mutate(async () => {
                        await api.addBotGroupActor(bot!.uuid!, actorGroup!.group.id, externalActorID.trim(), externalActorID.trim(), actorName.trim() || undefined);
                        setActorGroup(null);
                    })}
                >
                    {t('botAccess.addController', {defaultValue: 'Add controller'})}
                </Button>
            </DialogActions>
        </Dialog>
    </Dialog>;
};

export default BotAccessDialog;
