import { ListSubheader, Stack, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontSizes } from '@tingly/ui/theme/fonts';
import SelectWithCustom from '@tingly/ui/components/SelectWithCustom';
import { formatDurationShort } from '@tingly/ui/utils/duration';
import type { Thread } from '../../client';
import { AutoFixHigh, ExpandMore } from '../icons';
import { composerPickerLabelSx, composerPickerMenuProps, composerPickerSx } from '../composerPicker';

/** off, delay (the default wait), now, or `delay:<minutes>` (1 to 60) for a wait of the user's own. */
export type AutoAnswer = string;
const AUTO_ANSWERS = ['off', 'delay', 'now'];
/** What plain "delay" waits; typing it as a custom value stores "delay". */
const DEFAULT_DELAY_MINUTES = 5;
/** The longest custom wait, in minutes; the server takes the same bound. */
export const MAX_AUTO_ANSWER_MINUTES = 60;

/** The minutes of a custom `delay:<minutes>` setting; undefined for any other. */
export function customDelayMinutes(setting: string): number | undefined {
    const m = /^delay:([1-9]\d*)$/.exec(setting);
    const n = m ? Number(m[1]) : NaN;
    return n >= 1 && n <= MAX_AUTO_ANSWER_MINUTES ? n : undefined;
}

/**
 * The setting a thread's questions follow: its own, else delay for a project
 * thread (its owner is usually away) and off for a folder session (its owner
 * is usually watching). Mirrors the server's autoAnswerMode.
 */
export function autoAnswerOf(thread: Thread, folder = false): AutoAnswer {
    const own = thread.auto_answer;
    if (own && (AUTO_ANSWERS.includes(own) || customDelayMinutes(own) !== undefined)) return own;
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
    const duration = (m: string) => formatDurationShort((customDelayMinutes(m) ?? 0) * 60);
    const options = AUTO_ANSWERS.map((m) => ({ value: m, label: t(`autoAnswer.${m}`), secondary: t(`autoAnswer.${m}Help`) }));
    return (
        <SelectWithCustom<string>
            disabled={disabled}
            variant="standard"
            disableUnderline
            IconComponent={ExpandMore}
            inputProps={{ 'aria-label': t('autoAnswer.title') }}
            value={mode}
            onChange={(m) => { if (!disabled) onChange(m); }}
            options={options}
            menuHeader={(
                <ListSubheader key="header" sx={{ lineHeight: 1.4, pt: 0.5, pb: 0.75, bgcolor: 'transparent', fontSize: fontSizes.sm, color: 'text.secondary' }}>
                    {t('autoAnswer.menuTitle')}
                </ListSubheader>
            )}
            custom={{
                label: t('autoAnswer.custom'),
                dialogTitle: t('autoAnswer.customTitle'),
                inputLabel: t('autoAnswer.customInput'),
                helperText: t('autoAnswer.customHelp', { max: MAX_AUTO_ANSWER_MINUTES }),
                unit: t('autoAnswer.minutes'),
                inputMode: 'numeric',
                confirmLabel: t('autoAnswer.customApply'),
                cancelLabel: t('autoAnswer.customCancel'),
                parse: (raw) => {
                    const n = /^\d+$/.test(raw) ? Number(raw) : NaN;
                    if (!(n >= 1 && n <= MAX_AUTO_ANSWER_MINUTES)) return { error: t('autoAnswer.customInvalid', { max: MAX_AUTO_ANSWER_MINUTES }) };
                    return { value: n === DEFAULT_DELAY_MINUTES ? 'delay' : `delay:${n}` };
                },
                toInput: (m) => String(customDelayMinutes(m) ?? ''),
                format: (m) => t('autoAnswer.delayCustom', { duration: duration(m) }),
            }}
            renderValue={(m) => (
                <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center' }}>
                    <AutoFixHigh />
                    <Typography noWrap sx={composerPickerLabelSx}>
                        {AUTO_ANSWERS.includes(m) ? t(`autoAnswer.${m}Label`) : t('autoAnswer.delayCustomLabel', { duration: duration(m) })}
                    </Typography>
                </Stack>
            )}
            sx={composerPickerSx}
            MenuProps={{ ...composerPickerMenuProps, slotProps: { paper: { sx: { maxWidth: 340 } } } }}
        />
    );
}
