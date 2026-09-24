import {Archive, ArrowBack} from '@/components/icons';
import type {MessageInfo, SessionInfo} from '@/services/deskApi';
import {Alert, Box, Chip, IconButton, Stack, Tooltip, Typography} from '@mui/material';
import {useEffect, useMemo, useRef} from 'react';
import {useTranslation} from 'react-i18next';
import Composer from './Composer';
import {buildTranscript, isBusyStatus, pendingRequestId, sessionTitle} from './deskUtils';
import FolderChip from './FolderChip';
import PermissionModeSelect from './PermissionModeSelect';
import Transcript from './Transcript';

interface SessionViewProps {
    session: SessionInfo;
    messages: MessageInfo[];
    permissionModes: string[];
    onSend: (text: string) => Promise<boolean>;
    onRespond: (requestId: string, approved: boolean, answer: string) => Promise<void>;
    onInterrupt: () => Promise<void>;
    onArchive: () => Promise<void>;
    onPermissionModeChange: (mode: string) => Promise<void>;
    // Set on narrow screens, where the session list is a separate view.
    onBack?: () => void;
}

const COLUMN_MAX_WIDTH = 760;

const SessionView = ({
    session, messages, permissionModes, onSend, onRespond, onInterrupt, onArchive, onPermissionModeChange, onBack,
}: SessionViewProps) => {
    const {t} = useTranslation();
    const scrollRef = useRef<HTMLDivElement>(null);

    const blocks = useMemo(() => buildTranscript(messages), [messages]);
    const turnInFlight = isBusyStatus(session.status);
    const pendingId = pendingRequestId(blocks, turnInFlight);
    const isClosed = session.status === 'closed';

    // Follow new output, but only if the reader is already at the bottom:
    // scrolling up to read earlier output must not be yanked back down.
    const stickToBottom = useRef(true);
    useEffect(() => {
        const el = scrollRef.current;
        if (el && stickToBottom.current) el.scrollTop = el.scrollHeight;
    }, [blocks]);
    useEffect(() => {
        stickToBottom.current = true;
    }, [session.id]);

    return (
        <Box sx={{display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0}}>
            <Stack
                direction="row"
                spacing={1}
                sx={{alignItems: 'center', px: 2, py: 1.25, borderBottom: 1, borderColor: 'divider', minWidth: 0}}
            >
                {onBack && (
                    <IconButton size="small" onClick={onBack} aria-label={t('common.back', {defaultValue: 'Back'})}>
                        <ArrowBack fontSize="small"/>
                    </IconButton>
                )}
                <Typography variant="subtitle1" noWrap sx={{fontWeight: 600, minWidth: 0, color: 'text.primary'}}>{sessionTitle(session)}</Typography>
                <FolderChip path={session.project}/>
                {session.status === 'failed' && <Chip size="small" color="error" variant="outlined" label={t('desk.statusFailed', {defaultValue: 'failed'})}/>}
                {isClosed && <Chip size="small" variant="outlined" label={t('desk.statusArchived', {defaultValue: 'archived'})}/>}
                <Box sx={{flex: 1}}/>
                {!isClosed && (
                    <Tooltip title={t('desk.archiveHint', {defaultValue: 'Archive — ends the session; the folder and history stay'})}>
                        <IconButton size="small" onClick={onArchive} aria-label={t('desk.archive', {defaultValue: 'Archive'})}><Archive fontSize="small"/></IconButton>
                    </Tooltip>
                )}
            </Stack>

            <Box
                ref={scrollRef}
                onScroll={(e) => {
                    const el = e.currentTarget;
                    stickToBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
                }}
                sx={{flex: 1, minHeight: 0, overflowY: 'auto', px: 2}}
            >
                <Box sx={{maxWidth: COLUMN_MAX_WIDTH, mx: 'auto', py: 3}}>
                    <Transcript blocks={blocks} pendingRequestId={pendingId} working={turnInFlight} onRespond={onRespond}/>
                </Box>
            </Box>

            <Box sx={{px: 2, pb: 2, pt: 1}}>
                <Box sx={{maxWidth: COLUMN_MAX_WIDTH, mx: 'auto'}}>
                    {isClosed ? (
                        <Alert severity="info" variant="outlined">
                            {t('desk.archived', {defaultValue: 'This session is archived — the folder and history stay, but it can no longer be steered.'})}
                        </Alert>
                    ) : (
                        <Composer
                            key={session.id}
                            disabled={turnInFlight}
                            onStop={turnInFlight ? onInterrupt : undefined}
                            placeholder={turnInFlight
                                ? (pendingId
                                    ? t('desk.waitingForYou', {defaultValue: 'Waiting for your answer above…'})
                                    : t('desk.turnInProgress', {defaultValue: 'Working… stop it to send something else'}))
                                : t('desk.messagePlaceholder', {defaultValue: 'Reply…'})}
                            onSubmit={onSend}
                            minRows={1}
                            context={(
                                <PermissionModeSelect
                                    value={session.permission_mode}
                                    permissionModes={permissionModes}
                                    onChange={(m) => void onPermissionModeChange(m)}
                                />
                            )}
                        />
                    )}
                </Box>
            </Box>
        </Box>
    );
};

export default SessionView;
