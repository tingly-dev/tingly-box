import {MenuItem, Stack, TextField, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';
import LimitsEditor, {type Limits} from './LimitsEditor';

export interface PolicyDraft {
    limits: Limits;
    overflow: 'queue' | 'reject';
    queue_timeout_seconds?: number;
}

export const EMPTY_POLICY_DRAFT: PolicyDraft = {limits: {}, overflow: 'queue'};

interface PolicyLimitsFieldsProps {
    title?: string;
    hint?: string;
    value: PolicyDraft;
    onChange: (next: PolicyDraft) => void;
}

/**
 * Limits of a person / team (actor scope): caps plus what to do when the
 * concurrency cap is hit. Usage-window blocks always answer 429, so overflow
 * only concerns concurrency.
 */
export default function PolicyLimitsFields({title, hint, value, onChange}: PolicyLimitsFieldsProps) {
    const {t} = useTranslation();
    const hasConcurrency = !!value.limits.max_concurrent;
    return (
        <Stack spacing={1.5} data-testid="policy-limits-fields">
            {title && <Typography variant="subtitle2">{title}</Typography>}
            {hint && <Typography variant="body2" color="text.secondary">{hint}</Typography>}
            <LimitsEditor value={value.limits} onChange={(limits) => onChange({...value, limits})} showSoft={false}/>
            {hasConcurrency && (
                <Stack direction={{xs: 'column', sm: 'row'}} spacing={1.5}>
                    <TextField
                        select size="small" fullWidth label={t('limits.overflow.label')}
                        value={value.overflow}
                        onChange={(e) => onChange({...value, overflow: e.target.value as 'queue' | 'reject'})}
                        helperText={t('limits.overflow.hint')}
                    >
                        <MenuItem value="queue">{t('limits.overflow.queue')}</MenuItem>
                        <MenuItem value="reject">{t('limits.overflow.reject')}</MenuItem>
                    </TextField>
                    {value.overflow === 'queue' && (
                        <TextField
                            size="small" type="number" fullWidth label={t('limits.overflow.timeout')}
                            value={value.queue_timeout_seconds ? String(value.queue_timeout_seconds) : ''}
                            placeholder="15"
                            onChange={(e) => {
                                const n = Math.floor(Number(e.target.value));
                                onChange({...value, queue_timeout_seconds: e.target.value.trim() && n > 0 ? n : undefined});
                            }}
                            slotProps={{htmlInput: {min: 1, 'aria-label': t('limits.overflow.timeout')}}}
                        />
                    )}
                </Stack>
            )}
        </Stack>
    );
}
