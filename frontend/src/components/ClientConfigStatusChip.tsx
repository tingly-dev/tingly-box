// The Agent page's answer to "is my tool using what this page shows?" —
// see .design/agent-page-redesign.md §3.2. One persistent chip next to the
// agent's name instead of a toast that disappears: quiet when applied, a
// warning with the exact differences when out of date, and the next action
// (Auto Config) one click away in both non-applied states.
import { CheckCircle, WarningAmber, InfoOutlined } from '@/components/icons';
import { Box, Button, Stack, Tooltip, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { fontMono, fontSizes } from '@/theme/fonts';
import type { ClientConfigStatus } from '@/hooks/useClientConfigStatus';

interface Props {
    status: ClientConfigStatus | null;
    /** Opens the page's Auto Config. */
    onApply: () => void;
}

export const ClientConfigStatusChip: React.FC<Props> = ({ status, onApply }) => {
    const { t } = useTranslation();
    if (!status) return null;
    const path = status.path ?? '';
    const diffs = status.differences ?? [];

    if (status.state === 'applied') {
        return (
            <Tooltip title={t('clientConfigStatus.appliedTooltip', { path })} arrow>
                <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center', color: 'success.main' }}>
                    <CheckCircle sx={{ fontSize: 16 }} />
                    <Typography variant="caption" sx={{ fontWeight: 500 }}>{t('clientConfigStatus.applied')}</Typography>
                </Stack>
            </Tooltip>
        );
    }

    const outdated = status.state === 'outdated';
    const tooltip = outdated ? (
        <Box>
            <Typography variant="caption" sx={{ display: 'block', mb: 0.5 }}>{t('clientConfigStatus.outdatedTooltip', { path })}</Typography>
            {diffs.map(d => (
                <Box key={d.key} sx={{ fontFamily: fontMono, fontSize: 11, lineHeight: 1.5 }}>
                    {d.key}: {d.applied || '—'} → {d.expected || '—'}
                </Box>
            ))}
        </Box>
    ) : t('clientConfigStatus.notAppliedTooltip', { path });

    return (
        <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center' }}>
            <Tooltip title={tooltip} arrow>
                <Stack direction="row" spacing={0.5} sx={{ alignItems: 'center', color: outdated ? 'warning.main' : 'text.secondary' }}>
                    {outdated ? <WarningAmber sx={{ fontSize: 16 }} /> : <InfoOutlined sx={{ fontSize: 16 }} />}
                    <Typography variant="caption" sx={{ fontWeight: 500 }}>
                        {outdated ? t('clientConfigStatus.outdated', { count: diffs.length }) : t('clientConfigStatus.notApplied')}
                    </Typography>
                </Stack>
            </Tooltip>
            <Button size="small" onClick={onApply} sx={{ minWidth: 0, px: 1, py: 0, fontSize: fontSizes.sm, textTransform: 'none' }}>
                {outdated ? t('clientConfigStatus.reapply') : t('clientConfigStatus.apply')}
            </Button>
        </Stack>
    );
};
