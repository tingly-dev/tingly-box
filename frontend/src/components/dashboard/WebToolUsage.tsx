import { Paper, Stack, Table, TableBody, TableCell, TableContainer, TableHead, TableRow, Typography } from '@mui/material';
import { useTranslation } from 'react-i18next';
import type { AggregatedStat } from './ServiceStatsTable';

// Request-priced tools are not model requests: never feed these rows to token,
// cache, streaming or model-latency charts.
export default function WebToolUsage({ stats }: { stats: AggregatedStat[] }) {
    const { t } = useTranslation();
    if (!stats?.length) return null;
    const cost = stats.reduce((sum, row) => sum + (row.request_cost_cny || 0), 0);
    return (
        <Paper variant="outlined" sx={{ p: 2 }}>
            <Stack spacing={1}>
                <Typography variant="subtitle1">{t('dashboard.webToolSpend')} · CNY {cost.toFixed(3)}</Typography>
                <Typography variant="body2" color="text.secondary">{t('dashboard.webToolEstimateHint')}</Typography>
                <TableContainer>
                    <Table size="small" aria-label={t('dashboard.webToolSpend')}>
                        <TableHead><TableRow>
                            <TableCell>{t('dashboard.webToolSource')}</TableCell>
                            <TableCell>{t('dashboard.webToolOperation')}</TableCell>
                            <TableCell align="right">{t('dashboard.webToolCalls')}</TableCell>
                            <TableCell align="right">{t('dashboard.webToolBillable')}</TableCell>
                            <TableCell align="right">{t('dashboard.webToolErrors')}</TableCell>
                            <TableCell align="right">{t('dashboard.webToolEstimate')}</TableCell>
                        </TableRow></TableHead>
                        <TableBody>{stats.map((row) => <TableRow key={`${row.provider_uuid}/${row.model || row.key}`}>
                            <TableCell>{row.provider_name || row.provider_uuid}</TableCell>
                            <TableCell>{(row.model || row.key).replace(/^deepseek-tools\//, '')}</TableCell>
                            <TableCell align="right">{row.request_count}</TableCell>
                            <TableCell align="right">{row.billable_requests || 0}</TableCell>
                            <TableCell align="right">{row.error_count}</TableCell>
                            <TableCell align="right">CNY {(row.request_cost_cny || 0).toFixed(3)}</TableCell>
                        </TableRow>)}</TableBody>
                    </Table>
                </TableContainer>
            </Stack>
        </Paper>
    );
}
