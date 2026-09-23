import { useTranslation } from 'react-i18next';
import {
    Box,
    Chip,
    Grid,
    Paper,
    Skeleton,
    Stack,
    Typography,
} from '@mui/material';
import { Cloud, Server, Users } from '@/components/icons';
import { RosterBreakdownTable, RosterTopList } from '@/components/dashboard';
import type { AggregatedStat, MetricRow, RosterAxisState, UsageMetricLabels, ShareBarItem } from '@/components/dashboard';
import RosterDetailHeader from './RosterDetailHeader';
import { usageTableCardSx } from './RosterTable';
import type { UserUsageRow, ViewMode } from './userUsageModel';

type ActiveAxis =
    | RosterAxisState<UserUsageRow, AggregatedStat>
    | RosterAxisState<AggregatedStat, AggregatedStat>;

/**
 * The detail card below the roster: subject identity header + breakdown
 * table (per-account models, or accounts behind a model/provider), plus the
 * ranked Top list beside it. The breakdown table shares the roster card's
 * anatomy, so both tables read as one style.
 */
export default function RosterDetailSection({
    panelRef,
    viewMode,
    detailSubject,
    showDetailTop,
    activeAxis,
    selectedUser,
    selectedModel,
    selectedProvider,
    usageMetricLabels,
    accountDisplayName,
    detailShareItems,
}: {
    panelRef: React.Ref<HTMLDivElement>;
    viewMode: ViewMode;
    detailSubject: MetricRow | undefined;
    showDetailTop: boolean;
    activeAxis: ActiveAxis;
    selectedUser?: UserUsageRow;
    selectedModel?: AggregatedStat;
    selectedProvider?: AggregatedStat;
    usageMetricLabels: UsageMetricLabels;
    accountDisplayName: (userID: string) => string;
    detailShareItems: ShareBarItem[];
}) {
    const { t } = useTranslation();

    return (
        // Detail section: the breakdown table gets the exact same card
        // anatomy as the roster card above (header bar + count chip +
        // flush table), so both tables read as one style. Subject
        // identity lives in the card's header bar; the "All models [N]"
        // sub-header it used to have is the same info as the chip.
        <Grid
            container
            spacing={2}
            ref={panelRef}
            sx={{ alignItems: 'stretch', scrollMarginTop: { xs: 72, lg: 0 } }}
        >
            {detailSubject ? (<>
                <Grid size={{ xs: 12, lg: showDetailTop ? 9 : 12 }} sx={{ display: 'flex', minWidth: 0 }}>
                    <Paper elevation={0} sx={{ ...usageTableCardSx, display: 'flex', flexDirection: 'column' }}>
                        <Box
                            sx={{
                                minHeight: 72,
                                p: 2.5,
                                display: 'flex',
                                flexWrap: 'wrap',
                                alignItems: 'center',
                                justifyContent: 'space-between',
                                gap: 1.5,
                                borderBottom: '1px solid',
                                borderColor: 'divider',
                            }}
                        >
                            <RosterDetailHeader
                                viewMode={viewMode}
                                selectedUser={selectedUser}
                                selectedModel={selectedModel}
                                selectedProvider={selectedProvider}
                                t={t}
                            />
                            <Chip size="small" label={activeAxis.detail.length} sx={{ height: 22 }} />
                        </Box>
                        {activeAxis.detailLoading ? (
                            <Stack spacing={1.5} sx={{ overflow: 'hidden', p: 2.5 }}>
                                {Array.from({ length: 3 }).map((_, index) => <Skeleton key={index} variant="rounded" height={44} />)}
                            </Stack>
                        ) : viewMode === 'account' ? (
                            <RosterBreakdownTable
                                items={activeAxis.detail}
                                rowKey={(model) => `${model.provider_uuid}-${model.model || model.key}`}
                                identityColumns={[
                                    {
                                        key: 'provider',
                                        label: t('dashboard.userUsage.provider', { defaultValue: 'Provider' }),
                                        render: (model) => model.provider_name || '—',
                                    },
                                    {
                                        key: 'model',
                                        label: t('dashboard.userUsage.model', { defaultValue: 'Model' }),
                                        render: (model) => (
                                            <Typography variant="body2" sx={{ fontWeight: 600 }}>{model.model || model.key}</Typography>
                                        ),
                                    },
                                ]}
                                ariaLabel={t('dashboard.userUsage.allModels', { defaultValue: 'All models' })}
                                noUsageLabel={t('dashboard.userUsage.noUsage', { defaultValue: 'No usage in this period' })}
                                noUsageHint={t('dashboard.userUsage.noUsageHint', { defaultValue: 'The user remains listed because their access is registered.' })}
                                usageMetricLabels={usageMetricLabels}
                            />
                        ) : (
                            <RosterBreakdownTable
                                items={activeAxis.detail}
                                rowKey={(account) => account.user_id || account.key}
                                identityColumns={[
                                    {
                                        key: 'user',
                                        label: t('dashboard.userUsage.user', { defaultValue: 'User' }),
                                        render: (account) => (
                                            <>
                                                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                                                    {accountDisplayName(account.user_id || account.key)}
                                                </Typography>
                                                <Typography variant="caption" sx={{ color: 'text.secondary', fontFamily: 'monospace' }}>
                                                    {account.user_id || account.key}
                                                </Typography>
                                            </>
                                        ),
                                    },
                                ]}
                                ariaLabel={viewMode === 'model'
                                    ? t('dashboard.userUsage.accountsUsingModelTitle', { defaultValue: 'Accounts using this model' })
                                    : t('dashboard.userUsage.accountsUsingProviderTitle', { defaultValue: 'Accounts using this provider' })}
                                noUsageLabel={t('dashboard.userUsage.noUsage', { defaultValue: 'No usage in this period' })}
                                usageMetricLabels={usageMetricLabels}
                            />
                        )}
                    </Paper>
                </Grid>
                {showDetailTop && (
                    <Grid size={{ xs: 12, lg: 3, xl: 2.4 }} sx={{ display: 'flex', minWidth: 0 }}>
                        <RosterTopList
                            items={detailShareItems}
                            title={viewMode === 'account'
                                ? t('dashboard.userUsage.topModels', { defaultValue: 'Top models' })
                                : t('dashboard.userUsage.topAccounts', { defaultValue: 'Top accounts' })}
                            othersLabel={t('dashboard.userUsage.others', { defaultValue: 'Others' })}
                            emptyLabel={t('dashboard.userUsage.noUsage', { defaultValue: 'No usage in this period' })}
                        />
                    </Grid>
                )}
            </>) : (
                <Grid size={{ xs: 12 }} sx={{ display: 'flex' }}>
                    <Paper elevation={0} sx={{ ...usageTableCardSx, display: 'flex' }}>
                        <Box sx={{ width: '100%', py: 6, display: 'grid', placeItems: 'center', textAlign: 'center' }}>
                            <Box>
                                {viewMode === 'account'
                                    ? <Users sx={{ fontSize: 42, color: 'text.disabled', mb: 1 }} />
                                    : viewMode === 'model'
                                    ? <Server sx={{ fontSize: 42, color: 'text.disabled', mb: 1 }} />
                                    : <Cloud sx={{ fontSize: 42, color: 'text.disabled', mb: 1 }} />}
                                <Typography variant="body1">
                                    {viewMode === 'account'
                                        ? t('dashboard.userUsage.selectUser', { defaultValue: 'Select a user to see details.' })
                                        : viewMode === 'model'
                                        ? t('dashboard.userUsage.selectModel', { defaultValue: 'Select a model to see details.' })
                                        : t('dashboard.userUsage.selectProvider', { defaultValue: 'Select a provider to see details.' })}
                                </Typography>
                            </Box>
                        </Box>
                    </Paper>
                </Grid>
            )}
        </Grid>
    );
}
