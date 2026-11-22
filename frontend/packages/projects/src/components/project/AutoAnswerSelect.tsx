import { ListItemText, ListSubheader, MenuItem, Select, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@tingly/ui/theme/fonts';
import type { Thread } from '../../client';
import { AutoFixHigh, ExpandMore } from '../icons';
import { composerPickerLabelSx, composerPickerMenuProps, composerPickerSx } from '../composerPicker';

export type AutoAnswer = 'off' | 'delay' | 'now';
const AUTO_ANSWERS: AutoAnswer[] = ['off', 'delay', 'now'];

/**
 * The setting a thread's questions follow: its own, else delay for a
 * project thread (its owner is usually away) and off for a folder session
 * (its owner is usually watching). Mirrors the server's autoAnswerMode.
 */
export function autoAnswerOf(thread: Thread, folder = false): AutoAnswer {
    const own = thread.auto_answer as AutoAnswer | undefined;
    if (own && AUTO_ANSWERS.includes(own)) return own;
    return folder ? 'off' : 'delay';
}

interface Props {
    thread: Thread;
    /** The thread sits in a working folder (not a project's thread). */
    folder?: boolean;
    onChange: (mode: AutoAnswer) => void;
    disabled?: boolean;
}

/**
 * How the thread treats a question with a suggested answer, as a quiet picker
 * in the message box's strip beside the permission mode: both say how far the
 * thread goes without the user. Its label reads on its own ("auto-answer in
 * 5 min"); each choice says what it does.
 */
export function AutoAnswerSelect({ thread, folder = false, onChange, disabled }: Props) {
    const { t } = useTranslation('projects');
    const mode = autoAnswerOf(thread, folder);
    return (
        <Select
            disabled={disabled}
            size="small"
            variant="standard"
            disableUnderline
            IconComponent={ExpandMore}
            inputProps={{ 'aria-label': t('autoAnswer.title') }}
            value={mode}
            onChange={(e) => { if (!disabled) onChange(e.target.value as AutoAnswer); }}
            renderValue={(m) => (
                <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center' }}>
                    <AutoFixHigh />
                    <Typography noWrap sx={composerPickerLabelSx}>{t(`autoAnswer.${m}Label`)}</Typography>
                </Stack>
            )}
            sx={composerPickerSx}
            MenuProps={{ ...composerPickerMenuProps, slotProps: { paper: { sx: { maxWidth: 340 } } } }}
        >
            <ListSubheader sx={{ lineHeight: 1.4, pt: 0.5, pb: 0.75, bgcolor: 'transparent', fontSize: fontSizes.sm, color: 'text.secondary' }}>
                {t('autoAnswer.menuTitle')}
            </ListSubheader>
            {AUTO_ANSWERS.map((m) => (
                <MenuItem key={m} value={m} sx={{ alignItems: 'flex-start' }}>
                    <ListItemText primary={t(`autoAnswer.${m}`)} secondary={t(`autoAnswer.${m}Help`)}
                        slotProps={{ primary: { sx: { fontSize: fontSizes.base } }, secondary: { sx: { whiteSpace: 'normal', fontSize: fontSizes.sm } } }} />
                </MenuItem>
            ))}
        </Select>
    );
}
