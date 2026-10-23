import { useEffect, useState, type ReactNode } from 'react';
import { Box, Button, CircularProgress, IconButton, Stack, TextField, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { IconTextPlus } from '@tabler/icons-react';
import { CopyIconButton } from '@/components/CopyIconButton';
import { DeleteOutline, Palette, Refresh, tablerMui } from '@/components/icons';
import type { PromptRunBase } from './promptRunStore';

const TextPlus = tablerMui(IconTextPlus);

interface PromptRunCardProps {
    run: PromptRunBase;
    // What the run started from: a thumbnail beside the prompt (Image to
    // Prompt) or the description above it (Idea to Prompt).
    aside?: ReactNode;
    header?: ReactNode;
    pendingLabel: string;
    failedHint: string;
    onCommit: (id: string, prompt: string) => void;
    onUse: (prompt: string) => void;
    onSaveSnippet: (prompt: string) => void;
    onRetry: () => void;
    onRemove: (id: string) => void;
}

// Runs are kept across days: anything not from today says which day.
const runTimeLabel = (createdAt: number): string => {
    const created = new Date(createdAt);
    return created.toDateString() === new Date().toDateString()
        ? created.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
        : created.toLocaleString([], { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit' });
};

// One prompt-tool run: the prompt as an editable draft with the ways to hand
// it on. Each page wraps it in a memoised card of its own (the page
// re-renders on every keystroke, and the list keeps runs across days).
const PromptRunCard = ({ run, aside, header, pendingLabel, failedHint, onCommit, onUse, onSaveSnippet, onRetry, onRemove }: PromptRunCardProps) => {
    const { t } = useTranslation();
    const [draft, setDraft] = useState(run.prompt);
    useEffect(() => { setDraft(run.prompt); }, [run.prompt]);
    const commit = () => { if (draft !== run.prompt) onCommit(run.id, draft); };
    const time = runTimeLabel(run.createdAt);

    return (
        <Box sx={{ display: 'flex', gap: 1.5, p: 1.5, border: 1, borderColor: 'divider', borderRadius: 1.5, bgcolor: 'background.paper' }}>
            {aside}
            <Stack spacing={1} sx={{ flex: 1, minWidth: 0 }}>
                {header}
                {run.status === 'pending' ? (
                    <Stack direction="row" spacing={1} sx={{ alignItems: 'center', minHeight: 72, color: 'text.secondary' }}>
                        <CircularProgress size={16} />
                        <Typography variant="body2">{pendingLabel}</Typography>
                    </Stack>
                ) : run.status === 'failed' ? (
                    <Box sx={{ minHeight: 72 }}>
                        <Typography variant="body2" color="error" sx={{ wordBreak: 'break-word' }}>{run.error}</Typography>
                        <Typography variant="caption" sx={{ color: 'text.secondary' }}>{failedHint}</Typography>
                    </Box>
                ) : (
                    <TextField
                        multiline
                        fullWidth
                        size="small"
                        value={draft}
                        onChange={(event) => setDraft(event.target.value)}
                        onBlur={commit}
                        slotProps={{ htmlInput: { 'aria-label': t('imagePrompt.promptField', { defaultValue: 'Prompt' }) } }}
                        sx={{ '& .MuiInputBase-root': { fontSize: 13, lineHeight: 1.55 } }}
                    />
                )}
                <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center', flexWrap: 'wrap' }}>
                    <Typography variant="caption" sx={{ color: 'text.secondary', mr: 'auto', minWidth: 0 }} noWrap>
                        {run.model} · {time}
                    </Typography>
                    {run.status === 'completed' && (
                        <>
                            <CopyIconButton
                                value={draft}
                                size="small"
                                iconSize={16}
                                label={t('playground.copyPrompt', { defaultValue: 'Copy prompt' })}
                                copiedLabel={t('common.copied', { defaultValue: 'Copied' })}
                            />
                            <Tooltip title={t('imagePrompt.saveSnippet', { defaultValue: 'Keep as a snippet' })}>
                                <IconButton size="small" onClick={() => onSaveSnippet(draft)} aria-label={t('imagePrompt.saveSnippet', { defaultValue: 'Keep as a snippet' })}>
                                    <TextPlus sx={{ fontSize: 16 }} />
                                </IconButton>
                            </Tooltip>
                            <Button size="small" variant="outlined" startIcon={<Palette sx={{ fontSize: 16 }} />} onClick={() => onUse(draft)} sx={{ ml: 0.5 }}>
                                {t('imagePrompt.usePlayground', { defaultValue: 'Use in Playground' })}
                            </Button>
                        </>
                    )}
                    {run.status === 'failed' && (
                        <Button size="small" startIcon={<Refresh sx={{ fontSize: 16 }} />} onClick={onRetry}>
                            {t('imagePrompt.retry', { defaultValue: 'Retry' })}
                        </Button>
                    )}
                    <Tooltip title={t('imagePrompt.remove', { defaultValue: 'Remove' })}>
                        <IconButton size="small" onClick={() => onRemove(run.id)} aria-label={t('imagePrompt.remove', { defaultValue: 'Remove' })}>
                            <DeleteOutline sx={{ fontSize: 16 }} />
                        </IconButton>
                    </Tooltip>
                </Stack>
            </Stack>
        </Box>
    );
};

export default PromptRunCard;
