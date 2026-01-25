/**
 * Logs Page
 *
 * Displays request logs for debugging and monitoring
 */

import RequestLog from '@/components/RequestLog';
import { PageLayout } from '@/components/PageLayout';
import UnifiedCard from '@/components/UnifiedCard';
import { api } from '../services/api';

const Logs = () => {
    return (
        <PageLayout loading={false}>
            <UnifiedCard title="Request Logs" size="full">
                <RequestLog
                    getLogs={async (params) => {
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
        </PageLayout>
    );
};

export default Logs;
