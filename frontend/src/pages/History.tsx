import { Box, CircularProgress, Button, Stack } from '@mui/material';
import { useCallback, useEffect, useState } from 'react';
import CardGrid, { CardGridItem } from '../components/CardGrid';
import UnifiedCard from '../components/UnifiedCard';
import HistoryFilters from '../components/HistoryFilters';
import HistoryStats from '../components/HistoryStats';
import HistoryTable from '../components/HistoryTable';
import ActivityLog from '../components/ActivityLog';
import { api } from '../services/api';

const History = () => {
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

    const formatAction = (action: string) => {
        return action.replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase());
    };

    const formatDetails = (details: any) => {
        if (!details) return 'N/A';
        if (typeof details === 'string') return details;
        return Object.entries(details).map(([k, v]) => `${k}: ${v}`).join(', ');
    };

    const handleExportJSON = () => {
        const dataStr = JSON.stringify(filteredHistory, null, 2);
        downloadFile('history.json', dataStr, 'application/json');
    };

    const handleExportCSV = () => {
        const headers = ['Timestamp', 'Action', 'Success', 'Message', 'Details'];
        const csvContent = [
            headers.join(','),
            ...filteredHistory.map(entry => [
                new Date(entry.timestamp).toISOString(),
                entry.action,
                entry.success,
                `"${entry.message.replace(/"/g, '""')}"`,
                `"${formatDetails(entry.details).replace(/"/g, '""')}"`
            ].join(','))
        ].join('\n');

        downloadFile('history.csv', csvContent, 'text/csv');
    };

    const handleExportTXT = () => {
        const txtContent = filteredHistory.map(entry =>
            `[${new Date(entry.timestamp).toLocaleString()}] ${entry.success ? 'Success' : 'Failed'}: ${entry.action}: ${entry.message}`
        ).join('\n');

        downloadFile('history.txt', txtContent, 'text/plain');
    };

    const downloadFile = (filename: string, content: string, mimeType: string) => {
        const blob = new Blob([content], { type: mimeType });
        const url = URL.createObjectURL(blob);
        const a = document.createElement('a');
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
        URL.revokeObjectURL(url);
    };

    if (loading) {
        return (
            <Box display="flex" justifyContent="center" alignItems="center" minHeight="400px">
                <CircularProgress />
            </Box>
        );
    }

    return (
        <Box>
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
                <CardGridItem xs={12}>
                    <UnifiedCard
                        title="Activity Log & History"
                        subtitle={`${activityLog.length} recent activity entries`}
                        size="large"
                    >
                        <Stack spacing={2}>
                            <ActivityLog
                                activityLog={activityLog}
                                onLoadActivityLog={loadActivityLog}
                                onClearLog={clearLog}
                            />
                        </Stack>
                    </UnifiedCard>
                </CardGridItem>

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
        </Box>
    );
};

export default History;
