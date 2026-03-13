import SystemLogViewer from '@/components/SystemLogViewer';
import UnifiedCard from '@/components/UnifiedCard';

const SystemLogsPage = () => {
    // Placeholder API call - will be updated once swagger generates SystemLogsApi
    const getSystemLogs = async (params?: { limit?: number; level?: string; since?: string }) => {
        try {
            const queryParams = new URLSearchParams();
            if (params?.limit) queryParams.append('limit', params.limit.toString());
            if (params?.level) queryParams.append('level', params.level);
            if (params?.since) queryParams.append('since', params.since);

            const response = await fetch(`/api/v1/system/logs?${queryParams.toString()}`, {
                headers: {
                    'Authorization': `Bearer ${localStorage.getItem('user_auth_token') || ''}`,
                },
            });

            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }

            const data = await response.json();
            return {
                total: data.total || 0,
                logs: data.logs || [],
            };
        } catch (error: any) {
            console.error('Failed to get system logs:', error);
            return { total: 0, logs: [] };
        }
    };

    return (
        <UnifiedCard title="System Logs" size="full">
            <SystemLogViewer getLogs={getSystemLogs} />
        </UnifiedCard>
    );
};

export default SystemLogsPage;
