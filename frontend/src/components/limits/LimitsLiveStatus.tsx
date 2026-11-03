import {Chip, Stack, Typography} from '@mui/material';
import {useTranslation} from 'react-i18next';
import type {LimitStatusEntry} from './LimitsEditor';

const STATE_COLOR = {ok: 'success', hot: 'warning', blocked: 'error'} as const;

/** One-line live readout of a limit's current load. */
export default function LimitsLiveStatus({entry}: {entry?: LimitStatusEntry}) {
    const {t} = useTranslation();
    if (!entry) return null;
    const state = (entry.state in STATE_COLOR ? entry.state : 'ok') as keyof typeof STATE_COLOR;
    return (
        <Stack direction="row" spacing={1} sx={{alignItems: "center", flexWrap: "wrap"}} data-testid="limits-live">
            <Typography variant="caption" color="text.secondary">{t('limits.live')}</Typography>
            <Chip size="small" color={STATE_COLOR[state]} variant="outlined" label={t(`limits.state.${state}`)}/>
            <Typography variant="caption">{t('limits.liveInflight', {count: entry.inflight})}</Typography>
            {(entry.requests > 0 || entry.tokens > 0) && (
                <Typography variant="caption" color="text.secondary">
                    {t('limits.liveWindow', {requests: entry.requests, tokens: entry.tokens})}
                </Typography>
            )}
        </Stack>
    );
}
