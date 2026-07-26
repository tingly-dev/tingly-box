import {Send as SendIcon} from '@/components/icons';
import {api} from '@/services/api';
import {notify} from '@/utils/notify';
import {fontMono} from '@/theme/fonts';
import type {BotChat} from '@/types/bot';
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
// The chat_id picker is the live, channel-native id /notify needs (concept (b),
// not the chat_id_lock inbound restriction and not the scenarios route target).
// It's an Autocomplete over the chats the parent already loaded — one input that
// both picks from the list and accepts a pasted id when no chat is registered
// yet. See .design/bot-interaction-api.md and ux-principles #5/#11.
export interface NotifyTestDialogProps {
    open: boolean;
    botUUID: string;
    botName?: string;
    /** The bot's reachable chats — owned by the parent (BotNotifyGroup), so the
     *  dialog doesn't re-fetch what's already loaded one level up. */
    chats: BotChat[];
    /** Pre-fill the chat_id (e.g. when opened from a specific chat row). */
    initialChatID?: string;
    onClose: () => void;
}

const LEVELS = ['info', 'warn', 'error'] as const;
type Level = (typeof LEVELS)[number];

const NotifyTestDialog: React.FC<NotifyTestDialogProps> = ({open, botUUID, botName, chats, initialChatID, onClose}) => {
    const {t} = useTranslation();

    const [chatID, setChatID] = useState('');
    const [title, setTitle] = useState('');
    const [body, setBody] = useState('');
    const [level, setLevel] = useState<Level>('info');
    const [sending, setSending] = useState(false);

    // Reset the form each time the dialog opens. The chat list is owned by the
    // parent, so there's no fetch here.
    useEffect(() => {
        if (!open) return;
        setChatID(initialChatID ?? '');
        setTitle('');
        setBody('');
        setLevel('info');
    }, [open, initialChatID]);

    const canSend = chatID.trim() !== '' && body.trim() !== '' && !sending;

    const handleSend = useCallback(async () => {
        setSending(true);
        const result = await api.notifyBot(botUUID, {
            chat_id: chatID.trim(),
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
    }, [botUUID, chatID, title, body, level, onClose, t]);

    return (
        <Dialog open={open} onClose={sending ? undefined : onClose} fullWidth maxWidth="sm">
            <DialogTitle>
                {t('notify.test.title', {defaultValue: 'Send a test notification'})}
                {botName ? ` — ${botName}` : ''}
            </DialogTitle>
            <DialogContent>
                <Stack spacing={2} sx={{mt: 0.5}}>
                    {/* One chat_id control: pick from the list OR paste a value
                        when no chat is registered yet (freeSolo). Avoids the
                        two-controls-one-value trap. */}
                    <Autocomplete
                        size="small"
                        freeSolo
                        options={chats}
                        getOptionLabel={(c) => (typeof c === 'string' ? c : c.chat_id)}
                        value={chats.find((c) => c.chat_id === chatID) ?? (chatID ? chatID : null)}
                        onChange={(_e, value) => setChatID(typeof value === 'string' ? value : (value?.chat_id ?? ''))}
                        onInputChange={(_e, value) => setChatID(value)}
                        renderOption={(props, option) => {
                            const chat = typeof option === 'string' ? {chat_id: option} : option;
                            return (
                                <li {...props} key={chat.chat_id}>
                                    <Stack direction="row" spacing={1} sx={{alignItems: 'center'}}>
                                        <Typography component="span" sx={{fontFamily: fontMono}}>
                                            {chat.chat_id}
                                        </Typography>
                                        {chat.is_paired && (
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
                                label={t('notify.test.chatId', {defaultValue: 'Chat ID'})}
                                placeholder={chats.length === 0
                                    ? t('notify.test.chatIdPlaceholder', {defaultValue: 'No chats yet — paste a Chat ID'})
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
                        label={t('notify.test.bodyField', {defaultValue: 'Body'})}
                        value={body}
                        onChange={(e) => setBody(e.target.value)}
                        disabled={sending}
                        required
                    />
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
