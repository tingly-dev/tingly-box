import { useCallback, useEffect, useMemo, useState } from 'react';
import {
    Box,
    CircularProgress,
    FormControl,
    IconButton,
    InputLabel,
    MenuItem,
    Select,
    ToggleButton,
    ToggleButtonGroup,
    Tooltip,
} from '@mui/material';
import { useTranslation } from 'react-i18next';
import { Refresh } from '@/components/icons';
import PageHeader from '@/components/PageHeader';
import { QuotaHistoryView } from '@/components/dashboard';
import api from '@/services/api';

type TimeRange = 'today' | '7d' | '30d' | '90d';

interface Provider {
    uuid: string;
    name: string;
}

const RANGE_DAYS: Record<TimeRange, number> = {
    today: 1,
    '7d': 7,
    '30d': 30,
    '90d': 90,
};

const toLocalISOString = (date: Date): string => {
    const offset = -date.getTimezoneOffset();
    const sign = offset >= 0 ? '+' : '-';
    const pad = (value: number) => String(Math.floor(Math.abs(value))).padStart(2, '0');
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}` +
        `T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}` +
        `${sign}${pad(offset / 60)}:${pad(offset % 60)}`;
};

const buildTimeRange = (range: TimeRange) => {
    const now = new Date();
    const start = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    start.setDate(start.getDate() - (RANGE_DAYS[range] - 1));
    return { startTime: toLocalISOString(start), endTime: toLocalISOString(now) };
};

export default function QuotaHistoryPage() {
    const { t } = useTranslation();
    const [range, setRange] = useState<TimeRange>('7d');
    const [provider, setProvider] = useState('all');
    const [providers, setProviders] = useState<Provider[]>([]);
    const [refreshKey, setRefreshKey] = useState(0);
    const [refreshing, setRefreshing] = useState(false);

    const loadProviders = useCallback(async () => {
        const result = await api.getProviders();
        if (result?.success && Array.isArray(result.data)) {
            setProviders(result.data.map(({ uuid, name }: Provider) => ({ uuid, name })));
        }
    }, []);

    useEffect(() => {
        loadProviders().catch((error) => console.error('Failed to load quota history providers:', error));
    }, [loadProviders]);

    const timeRange = useMemo(() => buildTimeRange(range), [range]);

    const refresh = async () => {
        setRefreshing(true);
        try {
            await loadProviders();
        } catch (error) {
            console.error('Failed to refresh quota history providers:', error);
        } finally {
            setRefreshKey((key) => key + 1);
            setRefreshing(false);
        }
    };

    return (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
            <PageHeader
                title={t('dashboard.quotaHistory.title', { defaultValue: 'Quota history' })}
                subtitle={t('dashboard.quotaHistory.pageSubtitle', {
                    defaultValue: 'Track how provider allowances and balances changed over time.',
                })}
                actions={
                    <>
                        <FormControl size="small" sx={{ minWidth: 180 }}>
                            <InputLabel>{t('dashboard.overview.provider', { defaultValue: 'Provider' })}</InputLabel>
                            <Select
                                value={provider}
                                label={t('dashboard.overview.provider', { defaultValue: 'Provider' })}
                                onChange={(event) => setProvider(event.target.value)}
                            >
                                <MenuItem value="all">{t('dashboard.overview.allProviders', { defaultValue: 'All providers' })}</MenuItem>
                                {providers.map((item) => (
                                    <MenuItem key={item.uuid} value={item.uuid}>{item.name}</MenuItem>
                                ))}
                            </Select>
                        </FormControl>
                        <ToggleButtonGroup
                            size="small"
                            exclusive
                            value={range}
                            onChange={(_, value: TimeRange | null) => value && setRange(value)}
                            aria-label={t('dashboard.userUsage.timeRange', { defaultValue: 'Time range' })}
                        >
                            <ToggleButton value="today">{t('layout.today')}</ToggleButton>
                            <ToggleButton value="7d">7D</ToggleButton>
                            <ToggleButton value="30d">30D</ToggleButton>
                            <ToggleButton value="90d">90D</ToggleButton>
                        </ToggleButtonGroup>
                        <Tooltip title={t('common.refresh', { defaultValue: 'Refresh' })}>
                            <span>
                                <IconButton onClick={refresh} disabled={refreshing} aria-label={t('common.refresh', { defaultValue: 'Refresh' })}>
                                    {refreshing ? <CircularProgress size={20} /> : <Refresh />}
                                </IconButton>
                            </span>
                        </Tooltip>
                    </>
                }
            />

            <QuotaHistoryView
                startTime={timeRange.startTime}
                endTime={timeRange.endTime}
                provider={provider}
                refreshKey={refreshKey}
                showHeading={false}
            />
        </Box>
    );
}
