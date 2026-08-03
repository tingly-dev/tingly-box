import {Send as SendIcon} from '@/components/icons';
import {api} from '@/services/api';
import {notify} from '@/utils/notify';
import {fontMono} from '@/theme/fonts';
import type {NotifyTarget} from '@/types/bot';
import {MARKDOWN_STRESS_BODY} from '@/components/notify/markdownStressBody';
import {
    Autocomplete,
    Button,
    CircularProgress,
    Dialog,
    DialogActions,
    DialogContent,
    DialogTitle,
    FormControl,
    InputLabel,
    MenuItem,
    Select,
    Stack,
    TextField,
    Typography,
} from '@mui/material';
import {useCallback, useEffect, useState} from 'react';
import {useTranslation} from 'react-i18next';

// NotifyTestDialog lets an operator send a one-way test notification to a
// running bot's chat — the concrete payoff of the notify usage guide. It drives
// POST /api/v1/bots/:bot/notify (api.notifyBot) end-to-end so the link is
// verifiable from the UI. One-way only; interact (request→reply) is out of scope
// for this pass.
//
// The picker displays the channel-native id users recognize, while submitting
// the selected chat's stable internal UUID to /notify.
export interface NotifyTestDialogProps {
    open: boolean;
    botUUID: string;
    botName?: string;
    /** The bot's reachable chats — owned by the parent (BotNotifyGroup), so the
     *  dialog doesn't re-fetch what's already loaded one level up. */
    targets: NotifyTarget[];
    /** Pre-fill the stable target UUID when opened from a specific row. */
    initialTargetID?: string;
    onClose: () => void;
}

const LEVELS = ['info', 'warn', 'error'] as const;
type Level = (typeof LEVELS)[number];

const NotifyTestDialog: React.FC<NotifyTestDialogProps> = ({open, botUUID, botName, targets, initialTargetID, onClose}) => {
    const {t} = useTranslation();

    const [targetID, setTargetID] = useState('');
    const [title, setTitle] = useState('');
    const [body, setBody] = useState('');
    const [level, setLevel] = useState<Level>('info');
    const [sending, setSending] = useState(false);

    // Reset the form each time the dialog opens. The chat list is owned by the
    // parent, so there's no fetch here. The Body defaults to the markdown
    // stress document so one click verifies platform rendering; the operator
    // can still clear or edit it.
    useEffect(() => {
        if (!open) return;
        setTargetID(initialTargetID ?? '');
        setTitle('');
        setBody(MARKDOWN_STRESS_BODY);
        setLevel('info');
    }, [open, initialTargetID]);

    const canSend = targets.some((target) => target.id === targetID) && body.trim() !== '' && !sending;

    const handleSend = useCallback(async () => {
        setSending(true);
        const selected = targets.find((target) => target.id === targetID);
        if (!selected) {
            setSending(false);
            notify.error(t('notify.test.pickKnownChat', {defaultValue: 'Select a discovered chat first'}));
            return;
        }
        const result = await api.notifyBot(botUUID, {
            target: {kind: selected.kind, id: selected.id},
            title: title.trim() || undefined,
            body: body.trim(),
            level,
        });
        setSending(false);
        if (result.error) {
            notify.error(t('notify.test.sendFailed', {defaultValue: 'Send failed: {{error}}', error: result.error}));
            return;
        }
        notify.success(t('notify.test.sent', {defaultValue: 'Notification sent'}));
        onClose();
    }, [botUUID, targetID, title, body, level, onClose, t, targets]);

    return (
        <Dialog open={open} onClose={sending ? undefined : onClose} fullWidth maxWidth="sm">
            <DialogTitle>
                {t('notify.test.title', {defaultValue: 'Send a test notification'})}
                {botName ? ` — ${botName}` : ''}
            </DialogTitle>
            <DialogContent>
                <Stack spacing={2} sx={{mt: 0.5}}>
                    {/* Display the concrete platform id, but submit the stable
                        internal DirectChat UUID selected from this list. */}
                    <Autocomplete
                        size="small"
                        options={targets}
                        getOptionLabel={(target) => target.name || target.external_id}
                        value={targets.find((target) => target.id === targetID) ?? null}
                        onChange={(_e, value) => setTargetID(value?.id ?? '')}
                        renderOption={(props, option) => {
                            const target = option;
                            const {key, ...optionProps} = props;
                            return (
                                <li key={key ?? target.id} {...optionProps}>
                                    <Stack direction="row" spacing={1} sx={{alignItems: 'center'}}>
                                        <Typography component="span" sx={{fontFamily: fontMono}}>
                                            {target.external_id}
                                        </Typography>
                                        <Typography component="span" variant="caption" color="text.secondary">
                                            {target.kind === 'group' ? t('notify.target.group', {defaultValue: 'Group'}) : t('notify.target.direct', {defaultValue: 'Direct'})}
                                        </Typography>
                                        {target.is_paired && (
                                            <Typography component="span" variant="caption" sx={{color: 'success.main'}}>
                                                {t('notify.test.paired', {defaultValue: 'paired'})}
                                            </Typography>
                                        )}
                                    </Stack>
                                </li>
                            );
                        }}
                        renderInput={(params) => (
                            <TextField
                                {...params}
                                label={t('notify.test.target', {defaultValue: 'Target'})}
                                placeholder={targets.length === 0
                                    ? t('notify.test.targetPlaceholder', {defaultValue: 'No authorized targets yet'})
                                    : ''}
                            />
                        )}
                    />
                    <TextField
                        size="small"
                        fullWidth
                        label={t('notify.test.titleField', {defaultValue: 'Title (optional)'})}
                        value={title}
                        onChange={(e) => setTitle(e.target.value)}
                        disabled={sending}
                    />
                    <TextField
                        size="small"
                        fullWidth
                        multiline
                        minRows={3}
                        label={t('notify.test.bodyField', {defaultValue: 'Body (markdown)'})}
                        value={body}
                        onChange={(e) => setBody(e.target.value)}
                        disabled={sending}
                        required
                    />
                    <Button
                        size="small"
                        onClick={() => setBody(MARKDOWN_STRESS_BODY)}
                        disabled={sending}
                        sx={{alignSelf: 'flex-start'}}
                    >
                        {t('notify.test.resetMarkdown', {defaultValue: 'Reset to markdown sample'})}
                    </Button>
                    <FormControl size="small" fullWidth disabled={sending}>
                        <InputLabel>{t('notify.test.level', {defaultValue: 'Level'})}</InputLabel>
                        <Select
                            value={level}
                            label={t('notify.test.level', {defaultValue: 'Level'})}
                            onChange={(e) => setLevel(e.target.value as Level)}
                        >
                            {LEVELS.map((l) => (
                                <MenuItem key={l} value={l}>{l}</MenuItem>
                            ))}
                        </Select>
                    </FormControl>
                </Stack>
            </DialogContent>
            <DialogActions sx={{px: 3, pb: 2}}>
                <Button onClick={onClose} disabled={sending}>
                    {t('common.cancel', {defaultValue: 'Cancel'})}
                </Button>
                <Button
                    variant="contained"
                    color="primary"
                    onClick={handleSend}
                    disabled={!canSend}
                    startIcon={sending ? undefined : <SendIcon/>}
                >
                    {sending ? <CircularProgress size={16} color="inherit"/> : t('notify.test.send', {defaultValue: 'Send'})}
                </Button>
            </DialogActions>
        </Dialog>
    );
};

export default NotifyTestDialog;
