import {
    Paper,
    Typography,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    TablePagination,
    Box,
    alpha,
    useTheme,
} from '@mui/material';
import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { hasCacheWrites } from './chartStyles';
import {
    UsageMetricHeaderCells,
    UsageMetricValueCells,
} from './UsageMetricCells';
import { getUsageMetricColumns } from './usageMetricColumns';
import { fontSizes } from '@/theme/fonts';
import { ProviderLabel } from '@/components/ProviderLogo';

export interface AggregatedStat {
    key: string;
    provider_uuid?: string;
    provider_name?: string;
    model?: string;
    scenario?: string;
    user_id?: string;
    request_count: number;
    total_tokens?: number;
    total_input_tokens: number;
    total_output_tokens: number;
    cache_read_tokens?: number;
    cache_write_tokens?: number;
    reasoning_tokens?: number;
    avg_latency_ms?: number;
    error_count?: number;
    error_rate?: number;
    streamed_count?: number;
}

interface ServiceStatsTableProps {
    stats: AggregatedStat[];
}

export default function ServiceStatsTable({ stats }: ServiceStatsTableProps) {
    const { t } = useTranslation();
    const showCacheWrite = hasCacheWrites(stats);
    const theme = useTheme();
    const [page, setPage] = useState(0);
    const [rowsPerPage, setRowsPerPage] = useState(10);

    // Get theme-aware empty icon background
    const getEmptyIconBg = () => {
        const palette = theme.palette as any;
        if (palette.isSunlit) return 'rgba(14, 165, 233, 0.1)';
        if (palette.mode === 'dark') return 'rgba(148, 163, 184, 0.1)';
        return 'rgba(100, 116, 139, 0.1)';
    };

    const handleChangePage = (_event: unknown, newPage: number) => {
        setPage(newPage);
    };

    const handleChangeRowsPerPage = (event: React.ChangeEvent<HTMLInputElement>) => {
        setRowsPerPage(parseInt(event.target.value, 10));
        setPage(0);
    };

    // Avoid a layout jump when reaching the last page with empty rows
    const emptyRows = page > 0 ? Math.max(0, (1 + page) * rowsPerPage - stats.length) : 0;

    const visibleStats = stats.slice(page * rowsPerPage, page * rowsPerPage + rowsPerPage);
    const columnCount = 2 + getUsageMetricColumns({ showCacheWrite }).length;

    return (
        <Paper
            elevation={0}
            sx={{
                borderRadius: 2,
                border: '1px solid',
                borderColor: 'divider',
                overflow: 'hidden',
                backgroundColor: 'background.paper',
                boxShadow: 'none',
            }}
        >
            <Box
                sx={{
                    p: 2.5,
                    borderBottom: '1px solid',
                    borderColor: 'divider',
                    display: 'flex',
                    justifyContent: 'space-between',
                    alignItems: 'center',
                }}
            >
                <Typography variant="h6" sx={{ fontWeight: 600, fontSize: '0.875rem' }}>
                    {t('dashboard.usageByModel.title', { defaultValue: 'Usage by Model' })}
                </Typography>
            </Box>
            <TableContainer sx={{ maxHeight: 600 }}>
                <Table stickyHeader>
                    <TableHead>
                        <TableRow
                            sx={{
                                backgroundColor: alpha(theme.palette.background.paper, 0.8),
                                '& .MuiTableCell-root': {
                                    fontWeight: 600,
                                    fontSize: fontSizes.sm,
                                    textTransform: 'uppercase',
                                    letterSpacing: '0.05em',
                                    color: 'text.secondary',
                                    py: 1.25,
                                    borderBottom: '1px solid',
                                    borderColor: 'divider',
                                },
                            }}
                        >
                            <TableCell sx={{ fontWeight: 600 }}>{t('dashboard.usageByModel.provider', { defaultValue: 'Provider' })}</TableCell>
                            <TableCell sx={{ fontWeight: 600 }}>{t('dashboard.usageByModel.model', { defaultValue: 'Model' })}</TableCell>
                            <UsageMetricHeaderCells showCacheWrite={showCacheWrite} />
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {stats.length === 0 ? (
                            <TableRow>
                                <TableCell colSpan={columnCount} align="center" sx={{ py: 8 }}>
                                    <Box sx={{ textAlign: 'center' }}>
                                        <Box
                                            sx={{
                                                width: 48,
                                                height: 48,
                                                borderRadius: 2,
                                                backgroundColor: getEmptyIconBg(),
                                                display: 'flex',
                                                alignItems: 'center',
                                                justifyContent: 'center',
                                                mb: 2,
                                                mx: 'auto',
                                            }}
                                        >
                                            <Box
                                                sx={{
                                                    width: 24,
                                                    height: 24,
                                                    borderRadius: '50%',
                                                    backgroundColor: 'text.disabled',
                                                    opacity: 0.3,
                                                }}
                                            />
                                        </Box>
                                        <Typography variant="body1" sx={{
                                            color: "text.secondary"
                                        }}>
                                            {t('dashboard.usageByModel.empty', { defaultValue: 'No usage data available' })}
                                        </Typography>
                                        <Typography
                                            variant="caption"
                                            sx={{
                                                color: "text.disabled",
                                                mt: 0.5
                                            }}>
                                            {t('dashboard.usageByModel.emptyHint', { defaultValue: 'Select a different time range or check back later' })}
                                        </Typography>
                                    </Box>
                                </TableCell>
                            </TableRow>
                        ) : (
                            <>
                                {visibleStats.map((stat, index) => (
                                    <TableRow
                                        key={index}
                                        hover
                                        sx={{
                                            transition: 'background-color 0.15s ease',
                                            '&:hover': {
                                                backgroundColor: 'action.hover',
                                            },
                                            '& .MuiTableCell-root': {
                                                py: 1.25,
                                                borderBottom: '1px solid',
                                                borderColor: 'divider',
                                            },
                                        }}
                                    >
                                        <TableCell>
                                            <ProviderLabel uuid={stat.provider_uuid} size={16}>
                                                <span>{stat.provider_name || '-'}</span>
                                            </ProviderLabel>
                                        </TableCell>
                                        <TableCell>
                                            <Typography
                                                variant="body2"
                                                sx={{
                                                    maxWidth: 200,
                                                    overflow: 'hidden',
                                                    textOverflow: 'ellipsis',
                                                    whiteSpace: 'nowrap',
                                                }}
                                                title={stat.model}
                                            >
                                                {stat.model || stat.key}
                                            </Typography>
                                        </TableCell>
                                        <UsageMetricValueCells
                                            usage={stat}
                                            showCacheWrite={showCacheWrite}
                                            cacheHitDigits={2}
                                            errorDigits={2}
                                            requestFormatter={(value) => value.toLocaleString()}
                                        />
                                    </TableRow>
                                ))}
                                {emptyRows > 0 && (
                                    <TableRow style={{ height: 53 * emptyRows }}>
                                        <TableCell colSpan={columnCount} />
                                    </TableRow>
                                )}
                            </>
                        )}
                    </TableBody>
                </Table>
            </TableContainer>
            <TablePagination
                rowsPerPageOptions={[5, 10, 25, 50]}
                component="div"
                count={stats.length}
                rowsPerPage={rowsPerPage}
                page={page}
                onPageChange={handleChangePage}
                onRowsPerPageChange={handleChangeRowsPerPage}
                sx={{
                    borderTop: '1px solid',
                    borderColor: 'divider',
                    '& .MuiTablePagination-toolbar': {
                        minHeight: 52,
                    },
                }}
            />
        </Paper>
    );
}
