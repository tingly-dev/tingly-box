import { useTranslation } from 'react-i18next';
import {
    Avatar,
    Box,
    Chip,
    Paper,
    Stack,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TablePagination,
    TableRow,
    TableSortLabel,
    ToggleButton,
    ToggleButtonGroup,
    Typography,
    alpha,
    useTheme,
} from '@mui/material';
import { ArrowForward } from '@/components/icons';
import SearchField from '@/components/SearchField';
import { UsageMetricValueCells } from '@/components/dashboard';
import type { AggregatedStat, MetricRow, RosterAxisState } from '@/components/dashboard';
import { formatDateTime, getModelKey, getProviderKey } from './userUsageModel';
import type { UserUsageRow, ViewMode, PrimaryColumn } from './userUsageModel';

// Shared card anatomy for the roster table and the detail card below it.
export const usageTableCardSx = {
    width: '100%',
    borderRadius: 2,
    border: '1px solid',
    borderColor: 'divider',
    backgroundColor: 'background.paper',
    boxShadow: 'none',
    overflow: 'hidden',
} as const;

type ActiveAxis =
    | RosterAxisState<UserUsageRow, AggregatedStat>
    | RosterAxisState<AggregatedStat, AggregatedStat>;

/**
 * The roster card: axis switcher + count chip + search header bar, the
 * three-axis roster table (account / model / provider), and its pagination.
 * Selection and scroll behavior are injected via `onSelectRow`; the axis
 * state itself comes from the page's three `useRosterAxis` instances through
 * `activeAxis` (only the active axis' rows are rendered, exactly as before).
 */
export default function RosterTable({
    viewMode,
    onViewModeChange,
    activeAxis,
    primaryColumns,
    showCacheWrite,
    rowsPerPage,
    onRowsPerPageChange,
    onSelectRow,
}: {
    viewMode: ViewMode;
    onViewModeChange: (mode: ViewMode) => void;
    activeAxis: ActiveAxis;
    primaryColumns: PrimaryColumn[];
    showCacheWrite: boolean;
    rowsPerPage: number;
    onRowsPerPageChange: (rowsPerPage: number) => void;
    onSelectRow: (key: string) => void;
}) {
    const { t } = useTranslation();
    const theme = useTheme();

    const rosterRowSx = (selected: boolean) => ({
        cursor: 'pointer',
        position: 'relative',
        transition: 'background-color 0.15s ease',
        '& .MuiTableCell-root': {
            py: 1.25,
            borderBottom: '1px solid',
            borderColor: 'divider',
        },
        '&.Mui-selected': {
            bgcolor: alpha(theme.palette.primary.main, 0.08),
            boxShadow: `inset 3px 0 0 ${theme.palette.primary.main}`,
            '&:hover': { bgcolor: alpha(theme.palette.primary.main, 0.12) },
        },
    });
    const selectionArrowSx = (selected: boolean) => ({ fontSize: 18, opacity: selected ? 1 : 0.22 });

    // Shared wrapper for all three axis branches — identical row anatomy
    // (selection styling, metric cells, arrow cell); only the identity cells
    // differ per axis.
    const renderRow = (key: string, selected: boolean, usage: MetricRow, identityCells: React.ReactNode) => (
        <TableRow
            key={key}
            hover
            selected={selected}
            onClick={() => onSelectRow(key)}
            sx={rosterRowSx(selected)}
        >
            {identityCells}
            <UsageMetricValueCells usage={usage} showTotal showCacheWrite={showCacheWrite} />
            <TableCell padding="checkbox">
                <ArrowForward sx={selectionArrowSx(selected)} color={selected ? 'primary' : 'inherit'} />
            </TableCell>
        </TableRow>
    );

    return (
        <Paper
            elevation={0}
            sx={{ ...usageTableCardSx, display: 'flex', flexDirection: 'column' }}
        >
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
                {/* Axis switcher lives on the table it controls
                    (conventional dashboard position), and doubles
                    as the card's title — a separate "All users"
                    caption would repeat it. */}
                <Stack direction="row" spacing={1.5} sx={{ alignItems: 'center' }}>
                    <ToggleButtonGroup
                        size="small"
                        exclusive
                        value={viewMode}
                        onChange={(_, value: ViewMode | null) => value && onViewModeChange(value)}
                        aria-label={t('dashboard.userUsage.viewMode', { defaultValue: 'View' })}
                        sx={{
                            '& .MuiToggleButton-root': {
                                px: 1.5,
                                py: 0.25,
                                fontSize: '0.78rem',
                                textTransform: 'none',
                            },
                        }}
                    >
                        <ToggleButton value="account">
                            {t('dashboard.userUsage.byAccount', { defaultValue: 'By account' })}
                        </ToggleButton>
                        <ToggleButton value="model">
                            {t('dashboard.userUsage.byModel', { defaultValue: 'By model' })}
                        </ToggleButton>
                        <ToggleButton value="provider">
                            {t('dashboard.userUsage.byProvider', { defaultValue: 'By provider' })}
                        </ToggleButton>
                    </ToggleButtonGroup>
                    <Chip
                        size="small"
                        label={activeAxis.visibleRows.length}
                        sx={{ height: 22 }}
                    />
                </Stack>
                <SearchField
                    value={activeAxis.search}
                    onChange={(event) => activeAxis.setSearch(event.target.value)}
                    placeholder={viewMode === 'account'
                        ? t('dashboard.userUsage.search', { defaultValue: 'Search users' })
                        : viewMode === 'model'
                        ? t('dashboard.userUsage.searchModels', { defaultValue: 'Search models' })
                        : t('dashboard.userUsage.searchProviders', { defaultValue: 'Search providers' })}
                    sx={{ width: { xs: '100%', sm: 220 } }}
                />
            </Box>
            <TableContainer
                sx={{
                    maxHeight: 520,
                    overscrollBehavior: 'contain',
                }}
            >
                <Table stickyHeader sx={{ minWidth: showCacheWrite ? 1080 : 980 }}>
                    <TableHead>
                        <TableRow
                            sx={{
                                backgroundColor: alpha(theme.palette.background.paper, 0.8),
                                '& .MuiTableCell-root': {
                                    fontWeight: 600,
                                    fontSize: '0.75rem',
                                    textTransform: 'uppercase',
                                    letterSpacing: '0.05em',
                                    color: 'text.secondary',
                                    py: 1.25,
                                    borderBottom: '1px solid',
                                    borderColor: 'divider',
                                },
                            }}
                        >
                            {primaryColumns.map((col, index) => (
                                <TableCell
                                    key={col.kind === 'sort' ? col.field : `label-${index}`}
                                    align={col.kind === 'sort' ? col.align : undefined}
                                    sortDirection={col.kind === 'sort' && activeAxis.sortField === col.field ? activeAxis.sortDirection : false}
                                >
                                    {col.kind === 'sort' ? (
                                        <TableSortLabel
                                            active={activeAxis.sortField === col.field}
                                            direction={activeAxis.sortField === col.field ? activeAxis.sortDirection : col.defaultDir}
                                            onClick={() => activeAxis.handleSort(col.field)}
                                        >
                                            {col.label}
                                        </TableSortLabel>
                                    ) : col.label}
                                </TableCell>
                            ))}
                            <TableCell padding="checkbox" />
                        </TableRow>
                    </TableHead>
                    <TableBody>
                        {viewMode === 'account' && (activeAxis.pagedRows as UserUsageRow[]).map((row) => renderRow(
                            row.token_id,
                            row.user_id === activeAxis.selectedKey,
                            row,
                            (
                                <TableCell>
                                    <Stack direction="row" spacing={1.25} sx={{ alignItems: 'center' }}>
                                        <Avatar sx={{
                                            width: 34,
                                            height: 34,
                                            fontSize: 14,
                                            bgcolor: row.user_id === activeAxis.selectedKey ? 'primary.main' : alpha(theme.palette.primary.main, 0.1),
                                            color: row.user_id === activeAxis.selectedKey ? 'primary.contrastText' : 'primary.main',
                                        }}>
                                            {row.display_name.slice(0, 1).toUpperCase()}
                                        </Avatar>
                                        <Box sx={{ minWidth: 0 }}>
                                            <Stack direction="row" spacing={0.75} sx={{ alignItems: 'center' }}>
                                                <Typography
                                                    variant="body1"
                                                    noWrap
                                                    sx={{ color: 'text.primary', fontWeight: 600 }}
                                                >
                                                    {row.display_name}
                                                </Typography>
                                                {row.account_type === 'primary' && (
                                                    <Chip
                                                        size="small"
                                                        color="primary"
                                                        variant="outlined"
                                                        label={t('dashboard.userUsage.primary', { defaultValue: 'Primary' })}
                                                        sx={{ height: 22 }}
                                                    />
                                                )}
                                                {!row.enabled && (
                                                    <Chip
                                                        size="small"
                                                        label={t('dashboard.userUsage.disabled', { defaultValue: 'Disabled' })}
                                                        sx={{ height: 22 }}
                                                    />
                                                )}
                                            </Stack>
                                            <Typography variant="body2">
                                                {row.account_type === 'primary'
                                                    ? t('dashboard.userUsage.globalToken', { defaultValue: 'Global model token' })
                                                    : row.last_used_at
                                                    ? t('dashboard.userUsage.lastUsed', {
                                                        value: formatDateTime(row.last_used_at),
                                                        defaultValue: `Last used ${formatDateTime(row.last_used_at)}`,
                                                    })
                                                    : t('dashboard.userUsage.neverUsed', { defaultValue: 'Never used' })}
                                            </Typography>
                                        </Box>
                                    </Stack>
                                </TableCell>
                            ),
                        ))}
                        {viewMode === 'model' && (activeAxis.pagedRows as AggregatedStat[]).map((row) => {
                            const key = getModelKey(row);
                            return renderRow(
                                key,
                                key === activeAxis.selectedKey,
                                row,
                                (
                                    <>
                                        <TableCell>{row.provider_name || '—'}</TableCell>
                                        <TableCell sx={{ fontWeight: 600 }}>{row.model || row.key}</TableCell>
                                    </>
                                ),
                            );
                        })}
                        {viewMode === 'provider' && (activeAxis.pagedRows as AggregatedStat[]).map((row) => {
                            const key = getProviderKey(row);
                            return renderRow(
                                key,
                                key === activeAxis.selectedKey,
                                row,
                                (<TableCell sx={{ fontWeight: 600 }}>{row.provider_name || row.key}</TableCell>),
                            );
                        })}
                        {activeAxis.visibleRows.length === 0 && (
                            <TableRow>
                                <TableCell
                                    colSpan={primaryColumns.length + 1}
                                    align="center"
                                    sx={{ py: 8 }}
                                >
                                    <Typography variant="body1" sx={{ color: 'text.secondary' }}>
                                        {viewMode === 'account'
                                            ? t('dashboard.userUsage.noUsers', { defaultValue: 'No users match your search.' })
                                            : viewMode === 'model'
                                            ? t('dashboard.userUsage.noModels', { defaultValue: 'No models match your search.' })
                                            : t('dashboard.userUsage.noProviders', { defaultValue: 'No providers match your search.' })}
                                    </Typography>
                                    <Typography variant="caption" sx={{ color: 'text.disabled', mt: 0.5, display: 'block' }}>
                                        {t('dashboard.userUsage.noUsersHint', { defaultValue: 'Try a different search term or time range.' })}
                                    </Typography>
                                </TableCell>
                            </TableRow>
                        )}
                    </TableBody>
                </Table>
            </TableContainer>
            {activeAxis.visibleRows.length > 0 && (
                <TablePagination
                    component="div"
                    count={activeAxis.visibleRows.length}
                    page={activeAxis.page}
                    onPageChange={(_, newPage) => activeAxis.setPage(newPage)}
                    rowsPerPage={rowsPerPage}
                    onRowsPerPageChange={(event) => onRowsPerPageChange(parseInt(event.target.value, 10))}
                    rowsPerPageOptions={[5, 10, 25, 50]}
                    sx={{ borderTop: '1px solid', borderColor: 'divider', flexShrink: 0 }}
                />
            )}
        </Paper>
    );
}
