import RequestLog from '@/components/RequestLog';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '@/services/api';

const LogsPage = () => {
    return (
        <UnifiedCard title="Request Logs" size="full">
            <RequestLog
                getLogs={async () => {
                    try {
                        const { logsApi } = await api.instances();
                        const response = await logsApi.apiV1LogGet();
                        return {
                            total: response.data.total || 0,
                            logs: response.data.logs || [],
                        };
                    } catch (error: any) {
                        console.error('Failed to get logs:', error);
                        return { total: 0, logs: [] };
                    }
                }}
                clearLogs={async () => {
                    try {
                        const { logsApi } = await api.instances();
                        await logsApi.apiV1LogDelete();
                        return { success: true, message: 'Logs cleared' };
                    } catch (error: any) {
                        console.error('Failed to clear logs:', error);
                        return { success: false, message: error.message || 'Failed to clear logs' };
                    }
                }}
            />
        </UnifiedCard>
    );
};

export default LogsPage;
