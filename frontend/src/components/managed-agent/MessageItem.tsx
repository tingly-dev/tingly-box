import {Build, Check, Close} from '@/components/icons';
import type {MessageInfo} from '@/services/managedAgentApi';
import {Alert, Box, Button, Paper, Stack, TextField, Typography} from '@mui/material';
import {useState} from 'react';
import {useTranslation} from 'react-i18next';

interface MessageItemProps {
    message: MessageInfo;
    pending: boolean;
    onRespond: (approved: boolean, answer: string) => void;
    responding: boolean;
}

const bubbleSx = (align: 'left' | 'right', color: string) => ({
    maxWidth: '85%',
    alignSelf: align === 'right' ? 'flex-end' : 'flex-start',
    bgcolor: color,
    px: 1.5,
    py: 1,
    borderRadius: 2,
});

// MessageItem renders one transcript entry. The transcript mixes chat text
// (Role set) with structured turn detail (Kind set) — thinking, tool calls,
// approvals — so this switches on Kind first, falling back to the plain
// user/assistant bubble. Only the single still-open approval/ask request
// (computed by the caller via findPendingRequest) renders its action
// buttons; every earlier one already shows its own resolved response entry.
const MessageItem = ({message, pending, onRespond, responding}: MessageItemProps) => {
    const {t} = useTranslation();
    const [answer, setAnswer] = useState('');
    const time = new Date(message.timestamp).toLocaleTimeString();

    if (message.kind === 'approval_request' || message.kind === 'ask_request') {
        const isAsk = message.kind === 'ask_request';
        return (
            <Paper variant="outlined" sx={{p: 1.5, borderColor: pending ? 'warning.main' : 'divider', bgcolor: pending ? 'warning.50' : 'transparent'}}>
                <Stack direction="row" spacing={1} sx={{mb: 0.5, alignItems: "center"}}>
                    <Build fontSize="small" color="warning"/>
                    <Typography variant="body2" sx={{fontWeight: 600}}>{message.content}</Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ml: 'auto'}}>{time}</Typography>
                </Stack>
                {message.payload != null && (
                    <Typography component="pre" variant="caption" sx={{fontFamily: 'monospace', whiteSpace: 'pre-wrap', color: 'text.secondary', m: 0}}>
                        {JSON.stringify(message.payload, null, 2)}
                    </Typography>
                )}
                {pending && (
                    <Stack direction="row" spacing={1} sx={{mt: 1, alignItems: "center"}}>
                        {isAsk && (
                            <TextField
                                size="small"
                                fullWidth
                                placeholder={t('managedAgent.answerPlaceholder', {defaultValue: 'Your answer'})}
                                value={answer}
                                onChange={(e) => setAnswer(e.target.value)}
                            />
                        )}
                        <Button size="small" variant="contained" color="success" startIcon={<Check/>} disabled={responding} onClick={() => onRespond(true, answer)}>
                            {t('managedAgent.approve', {defaultValue: 'Approve'})}
                        </Button>
                        <Button size="small" variant="outlined" color="error" startIcon={<Close/>} disabled={responding} onClick={() => onRespond(false, answer)}>
                            {t('managedAgent.deny', {defaultValue: 'Deny'})}
                        </Button>
                    </Stack>
                )}
            </Paper>
        );
    }

    if (message.kind === 'approval_response' || message.kind === 'ask_response') {
        return (
            <Typography variant="caption" color="text.secondary" sx={{fontStyle: 'italic'}}>
                {message.kind === 'approval_response'
                    ? t('managedAgent.wasAnswered', {defaultValue: '→ {{answer}}', answer: message.content})
                    : t('managedAgent.wasAnsweredWith', {defaultValue: '→ answered: {{answer}}', answer: message.content || '(empty)'})}
            </Typography>
        );
    }

    if (message.kind === 'error') {
        return <Alert severity="error" variant="outlined" sx={{py: 0}}>{message.content}</Alert>;
    }

    if (message.kind === 'system') {
        return <Typography variant="caption" color="text.secondary" sx={{fontStyle: 'italic'}}>{message.content}</Typography>;
    }

    if (message.kind === 'thinking') {
        return (
            <Typography variant="body2" color="text.secondary" sx={{fontStyle: 'italic', pl: 1, borderLeft: 2, borderColor: 'divider'}}>
                {message.content}
            </Typography>
        );
    }

    if (message.kind === 'tool_use' || message.kind === 'tool_result') {
        const isError = message.kind === 'tool_result' && (message.payload as {is_error?: boolean} | undefined)?.is_error;
        return (
            <Paper variant="outlined" sx={{p: 1, bgcolor: isError ? 'error.50' : 'action.hover'}}>
                <Stack direction="row" spacing={1} sx={{alignItems: "center"}}>
                    <Build fontSize="small" color={isError ? 'error' : 'action'}/>
                    <Typography variant="caption" color={isError ? 'error' : 'text.secondary'} sx={{fontWeight: 600}}>
                        {message.kind === 'tool_use' ? message.content : t('managedAgent.toolResult', {defaultValue: 'Result'})}
                    </Typography>
                </Stack>
                {message.kind === 'tool_use' && message.payload != null && (
                    <Typography component="pre" variant="caption" sx={{fontFamily: 'monospace', whiteSpace: 'pre-wrap', m: 0, mt: 0.5}}>
                        {JSON.stringify(message.payload, null, 2)}
                    </Typography>
                )}
                {message.kind === 'tool_result' && message.content && (
                    <Typography component="pre" variant="caption" sx={{fontFamily: 'monospace', whiteSpace: 'pre-wrap', m: 0, mt: 0.5, maxHeight: 200, overflow: 'auto', display: 'block'}}>
                        {message.content}
                    </Typography>
                )}
            </Paper>
        );
    }

    // Plain chat message: user or assistant.
    const isUser = message.role === 'user';
    return (
        <Box sx={{display: 'flex', flexDirection: 'column'}}>
            <Paper elevation={0} sx={bubbleSx(isUser ? 'right' : 'left', isUser ? 'primary.main' : 'action.hover')}>
                <Typography variant="body2" sx={{whiteSpace: 'pre-wrap', color: isUser ? 'primary.contrastText' : 'text.primary'}}>
                    {message.content}
                </Typography>
            </Paper>
        </Box>
    );
};

export default MessageItem;
