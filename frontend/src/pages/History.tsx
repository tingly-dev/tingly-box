import { Stack } from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import CardGrid from '../components/CardGrid';
import UnifiedCard from '../components/UnifiedCard';
import HistoryStats from '../components/HistoryStats';
import ActivityLog from '../components/ActivityLog';
import { PageLayout } from '../components/PageLayout';

const History = () => {
    const { t } = useTranslation();
    const [allHistory, setAllHistory] = useState<any[]>([]);
    const [filteredHistory, setFilteredHistory] = useState<any[]>([]);
    const [activityLog, setActivityLog] = useState<any[]>([]);
    const [loading, setLoading] = useState(false);

    // Filter state
    const [searchTerm, setSearchTerm] = useState('');
    const [filterType, setFilterType] = useState('all');
    const [filterStatus, setFilterStatus] = useState('all');
    
    // Auto refresh state
    const [autoRefresh, setAutoRefresh] = useState(false);
    const [refreshInterval, setRefreshInterval] = useState(60000); // 30 seconds

    // Stats
    const [stats, setStats] = useState({
        total: 0,
        success: 0,
        error: 0,
        today: 0,
    });

    const updateStats = useCallback((data: any[]) => {
        const total = data.length;
        const success = data.filter(entry => entry.success).length;
        const error = total - success;
        const today = new Date().toDateString();
        const todayCount = data.filter(entry =>
            new Date(entry.timestamp).toDateString() === today
        ).length;

        setStats({
            total,
            success,
            error,
            today: todayCount,
        });
    }, []);

    const applyFilters = useCallback(() => {
        const filtered = allHistory.filter(entry => {
            // Search filter
            if (searchTerm && !entry.action.toLowerCase().includes(searchTerm.toLowerCase()) &&
                !entry.message.toLowerCase().includes(searchTerm.toLowerCase())) {
                return false;
            }

            // Type filter
            if (filterType !== 'all' && entry.action !== filterType) {
                return false;
            }

            // Status filter
            if (filterStatus !== 'all' && entry.success.toString() !== filterStatus) {
                return false;
            }

            return true;
        });

        setFilteredHistory(filtered);
    }, [allHistory, searchTerm, filterType, filterStatus]);

    const loadHistory = useCallback(async () => {
        // setLoading(true);
        // const result = await api.getHistory(200);
        // if (result.success) {
        //     setAllHistory(result.data);
        //     updateStats(result.data);
        //     applyFilters();
        // }
        // setLoading(false);
    }, [updateStats, applyFilters]);

    const loadActivityLog = async () => {
        // const result = await api.getHistory(50);
        // if (result.success) {
        //     setActivityLog(result.data);
        // }
    };

    const clearLog = () => {
        setActivityLog([]);
    };

    // Initial load and filter updates
    useEffect(() => {
        loadHistory();
        loadActivityLog();
    }, [loadHistory]);

    useEffect(() => {
        applyFilters();
    }, [applyFilters]);

    // Auto refresh effect
    useEffect(() => {
        let interval: ReturnType<typeof setInterval> | null = null;
        if (autoRefresh) {
            interval = setInterval(loadHistory, refreshInterval);
        }
        return () => {
            if (interval) clearInterval(interval);
        };
    }, [autoRefresh, refreshInterval, loadHistory]);

    return (
        <PageLayout loading={loading}>
            <CardGrid>
                {/* Filter and Export Controls */}
                {/* <CardGridItem xs={12}>
                    <UnifiedCard
                        title="Filters & Controls"
                        subtitle="Search, filter, and export history data"
                        size="medium"
                    >
                        <HistoryFilters
                            searchTerm={searchTerm}
                            onSearchChange={setSearchTerm}
                            filterType={filterType}
                            onFilterTypeChange={setFilterType}
                            filterStatus={filterStatus}
                            onFilterStatusChange={setFilterStatus}
                            onRefresh={loadHistory}
                            onExport={handleExportJSON}
                            autoRefresh={autoRefresh}
                            onAutoRefreshChange={setAutoRefresh}
                            refreshInterval={refreshInterval}
                            onRefreshIntervalChange={setRefreshInterval}
                        />
                    </UnifiedCard>
                </CardGridItem> */}

                {/* Statistics */}
                <HistoryStats stats={stats} />

                {/* Activity Log - Enhanced */}
                <UnifiedCard
                    title={t('history.pageTitle')}
                    subtitle={t('history.subtitle', { count: activityLog.length })}
                    size="full"
                >
                    <Stack spacing={2}>
                        <ActivityLog
                            activityLog={activityLog}
                            onLoadActivityLog={loadActivityLog}
                            onClearLog={clearLog}
                        />
                    </Stack>
                </UnifiedCard>

                {/* History Table */}
                {/* <CardGridItem xs={12}>
                    <UnifiedCard
                        title="History Table"
                        subtitle={`${filteredHistory.length} filtered entries`}
                        size="large"
                    >
                        <HistoryTable
                            filteredHistory={filteredHistory}
                            onExportJSON={handleExportJSON}
                            onExportCSV={handleExportCSV}
                            onExportTXT={handleExportTXT}
                        />
                    </UnifiedCard>
                </CardGridItem> */}
            </CardGrid>
        </PageLayout>
    );
};

export default History;
