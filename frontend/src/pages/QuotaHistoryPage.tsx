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

type TimeRange = '5h' | '1d' | '7d' | '30d';

interface Provider {
    uuid: string;
    name: string;
}

const RANGE_MINUTES: Record<TimeRange, number> = {
    '5h': 5 * 60,
    '1d': 24 * 60,
    '7d': 7 * 24 * 60,
    '30d': 30 * 24 * 60,
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
    const start = new Date(now.getTime() - RANGE_MINUTES[range] * 60_000);
    return { startTime: toLocalISOString(start), endTime: toLocalISOString(now) };
};

export default function QuotaHistoryPage() {
    const { t } = useTranslation();
    const [range, setRange] = useState<TimeRange>('5h');
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

    const timeRange = useMemo(() => buildTimeRange(range), [range, refreshKey]);

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
                    defaultValue: "Today's samples and past days' quota highs and lows.",
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
                            <ToggleButton value="5h">5H</ToggleButton>
                            <ToggleButton value="1d">1D</ToggleButton>
                            <ToggleButton value="7d">7D</ToggleButton>
                            <ToggleButton value="30d">30D</ToggleButton>
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
            />
        </Box>
    );
}
