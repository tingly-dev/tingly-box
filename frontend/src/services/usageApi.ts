// Usage dashboard control-plane API: stats / timeseries / records / performance.
import {controlApi} from './openapi';

export const usageApi = {
    // Usage Dashboard API calls
    getUsageStats: async (params: {
        group_by?: string;
        start_time?: string;
        end_time?: string;
        provider?: string;
        model?: string;
        scenario?: string;
        user_id?: string;
        limit?: number;
        sort_by?: 'total_tokens' | 'request_count' | 'avg_latency';
        sort_order?: 'asc' | 'desc';
    } = {}): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/usage/stats', {
            headers,
            params: {
                query: {
                    group_by: params.group_by as any,
                    start_time: params.start_time,
                    end_time: params.end_time,
                    provider: params.provider,
                    model: params.model,
                    scenario: params.scenario,
                    user_id: params.user_id,
                    limit: params.limit,
                    sort_by: params.sort_by,
                    sort_order: params.sort_order,
                }
            }
        })),

    getUsageTimeSeries: async (params: {
        interval?: string;
        start_time?: string;
        end_time?: string;
        provider?: string;
        model?: string;
        scenario?: string;
        user_id?: string;
    } = {}): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/usage/timeseries', {
            headers,
            params: {
                query: {
                    interval: params.interval as any,
                    start_time: params.start_time,
                    end_time: params.end_time,
                    provider: params.provider,
                    model: params.model,
                    scenario: params.scenario,
                    user_id: params.user_id,
                } as any
            }
        })),

    getUsageRecords: async (params: {
        start_time?: string;
        end_time?: string;
        provider?: string;
        model?: string;
        scenario?: string;
        user_id?: string;
        status?: string;
        limit?: number;
        offset?: number;
    } = {}): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/usage/records', {
            headers,
            params: {
                query: {
                    start_time: params.start_time,
                    end_time: params.end_time,
                    provider: params.provider,
                    model: params.model,
                    scenario: params.scenario,
                    user_id: params.user_id,
                    status: params.status as any,
                    limit: params.limit,
                    offset: params.offset,
                } as any
            }
        })),

    getUsagePerformance: async (params: {
        start_time?: string;
        end_time?: string;
        provider?: string;
        model?: string;
        scenario?: string;
        user_id?: string;
    } = {}): Promise<any> =>
        controlApi((client, headers) => client.GET('/api/v1/usage/performance', {
            headers,
            params: {
                query: {
                    start_time: params.start_time,
                    end_time: params.end_time,
                    provider: params.provider,
                    model: params.model,
                    scenario: params.scenario,
                    user_id: params.user_id,
                }
            }
        })),
};
