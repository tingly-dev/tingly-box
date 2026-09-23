import { Box, Typography } from '@mui/material';
import type { AggregatedStat } from '@/components/dashboard';
import { formatDateTime } from './userUsageModel';
import type { UserUsageRow, ViewMode } from './userUsageModel';

// Identity line for the detail card's header bar. Single baseline row (name +
// id/provider + joined) so the header stays at the shared 72px minHeight and
// its bottom border aligns with the sibling Top card's — a stacked subtitle
// made this card visibly taller. Carries only what the selected roster row
// does NOT already show; status chip and "last used" live in that row.
export default function RosterDetailHeader({
    viewMode,
    selectedUser,
    selectedModel,
    selectedProvider,
    t,
}: {
    viewMode: ViewMode;
    selectedUser?: UserUsageRow;
    selectedModel?: AggregatedStat;
    selectedProvider?: AggregatedStat;
    t: (key: string, options?: Record<string, unknown>) => string;
}) {
    return (
        <Box sx={{ display: 'flex', flexWrap: 'wrap', alignItems: 'baseline', gap: 1.5, minWidth: 0 }}>
            <Typography variant="h6" noWrap sx={{ fontWeight: 650 }}>
                {viewMode === 'account'
                    ? selectedUser!.display_name
                    : viewMode === 'model'
                    ? (selectedModel!.model || selectedModel!.key)
                    : (selectedProvider!.provider_name || selectedProvider!.key)}
            </Typography>
            {viewMode !== 'provider' && (
                <Typography
                    variant="body2"
                    noWrap
                    sx={{ color: 'text.secondary', fontFamily: viewMode === 'account' ? 'monospace' : undefined }}
                >
                    {viewMode === 'account' ? selectedUser!.user_id : (selectedModel!.provider_name || '—')}
                </Typography>
            )}
            {viewMode === 'account' && selectedUser!.created_at && (
                <Typography variant="body2" noWrap sx={{ color: 'text.secondary' }}>
                    {t('dashboard.userUsage.joined', {
                        value: formatDateTime(selectedUser!.created_at),
                        defaultValue: `Added ${formatDateTime(selectedUser!.created_at)}`,
                    })}
                </Typography>
            )}
        </Box>
    );
}
