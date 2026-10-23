import { useState } from 'react';
import { Button, ListItemText, Menu, MenuItem, Stack, TextField, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Check, RestartAlt } from '@/components/icons';
import { matchStarter } from './instructionStarters';

interface InstructionsFieldProps<K extends string> {
    value: string;
    onChange: (next: string) => void;
    starters: Record<K, string>;
    defaultText: string;
    // i18n group holding `starters.<key>` and `starters.<key>Hint`.
    i18nGroup: 'imagePrompt' | 'ideaPrompt';
    onSubmit: () => void;
    canSubmit: boolean;
}

// The instructions a prompt tool sends with every run. They are the one thing
// on the page worth tuning, so they are an open field, not a setting behind a
// dialog. The starters fill it; they are not a mode.
function InstructionsField<K extends string>({ value, onChange, starters, defaultText, i18nGroup, onSubmit, canSubmit }: InstructionsFieldProps<K>) {
    const { t } = useTranslation();
    const [anchor, setAnchor] = useState<HTMLElement | null>(null);
    const starter = matchStarter(starters, value);
    const isDefault = value.trim() === defaultText;
    const label = t('imagePrompt.instructions', { defaultValue: 'Instructions for the model' });

    return (
        <Stack spacing={0.5} sx={{ flex: { lg: 1 }, minHeight: 0 }}>
            <Stack direction="row" sx={{ alignItems: 'center', minHeight: 28 }}>
                <Typography variant="caption" sx={{ color: 'text.secondary', mr: 'auto' }}>
                    {label}
                    {starter === null && ` · ${t('imagePrompt.edited', { defaultValue: 'edited' })}`}
                </Typography>
                <Button size="small" onClick={(event) => setAnchor(event.currentTarget)} sx={{ py: 0, minWidth: 0 }}>
                    {t('imagePrompt.startFrom', { defaultValue: 'Start from…' })}
                </Button>
                {!isDefault && (
                    <Button size="small" color="inherit" startIcon={<RestartAlt sx={{ fontSize: 16 }} />} onClick={() => onChange(defaultText)} sx={{ py: 0, ml: 0.5, color: 'text.secondary' }}>
                        {t('imagePrompt.resetInstructions', { defaultValue: 'Default' })}
                    </Button>
                )}
            </Stack>
            <TextField
                multiline
                fullWidth
                value={value}
                onChange={(event) => onChange(event.target.value)}
                onKeyDown={(event) => {
                    if ((event.metaKey || event.ctrlKey) && event.key === 'Enter' && canSubmit) {
                        event.preventDefault();
                        onSubmit();
                    }
                }}
                minRows={8}
                slotProps={{ htmlInput: { 'aria-label': label } }}
                sx={{
                    flex: { lg: 1 },
                    minHeight: 0,
                    display: 'flex',
                    '& .MuiInputBase-root': { flex: 1, alignItems: 'flex-start', overflow: 'auto', fontSize: 13, lineHeight: 1.55 },
                }}
            />
            <Menu anchorEl={anchor} open={Boolean(anchor)} onClose={() => setAnchor(null)}>
                {(Object.keys(starters) as K[]).map((key) => (
                    <MenuItem
                        key={key}
                        selected={starter === key}
                        onClick={() => { setAnchor(null); onChange(starters[key]); }}
                        sx={{ maxWidth: 360 }}
                    >
                        <ListItemText
                            primary={t(`${i18nGroup}.starters.${key}`)}
                            secondary={t(`${i18nGroup}.starters.${key}Hint`)}
                            slotProps={{ secondary: { sx: { whiteSpace: 'normal' } } }}
                        />
                        {starter === key && <Check sx={{ fontSize: 16, ml: 1, color: 'primary.main' }} />}
                    </MenuItem>
                ))}
            </Menu>
        </Stack>
    );
}

export default InstructionsField;
