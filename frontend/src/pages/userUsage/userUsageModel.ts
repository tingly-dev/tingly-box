import type { AggregatedStat, SortField, SortDirection } from '@/components/dashboard';
import api from '@/services/api';
import { toLocalISOString } from '@/utils/datetime';

export type TimeRange = 'today' | '7d' | '30d' | '90d';
export type ViewMode = 'account' | 'model' | 'provider';

export interface APITokenInfo {
    token_id: string;
    user_id: string;
    display_name: string;
    enabled: boolean;
    last_used_at?: string;
    created_at?: string;
    account_type?: 'primary' | 'sharing';
}

export interface UserUsageRow extends APITokenInfo {
    request_count: number;
    total_tokens: number;
    total_input_tokens: number;
    total_output_tokens: number;
    cache_read_tokens: number;
    cache_write_tokens: number;
    error_count: number;
    error_rate: number;
}

// A column either sorts the roster ('sort') or is a plain, non-sortable
// identity header ('label' — e.g. the Provider column on the model
// roster, where only Model is sortable). Keeping non-sortable columns in
// this same array means the header row and its colSpan never need a
// separate special case for "the model axis has one extra column".
export type PrimaryColumn =
    | { kind: 'sort'; field: SortField; label: string; align?: 'right'; defaultDir: SortDirection }
    | { kind: 'label'; label: string };

const RANGE_DAYS: Record<TimeRange, number> = {
    today: 1,
    '7d': 7,
    '30d': 30,
    '90d': 90,
};

export const buildTimeParams = (range: TimeRange) => {
    const now = new Date();
    const start = new Date(now.getFullYear(), now.getMonth(), now.getDate());
    if (range !== 'today') {
        start.setDate(start.getDate() - (RANGE_DAYS[range] - 1));
    }
    return {
        start_time: toLocalISOString(start),
        end_time: toLocalISOString(now),
    };
};

export const formatDateTime = (value?: string) => {
    if (!value) return '—';
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) return '—';
    return new Intl.DateTimeFormat(undefined, {
        month: 'short',
        day: 'numeric',
        hour: '2-digit',
        minute: '2-digit',
    }).format(date);
};

// Model name alone can collide across providers, so the model axis is keyed
// by provider + model together (mirrors the provider filter always paired
// with the model filter when querying the backend for one exact model).
export const getModelKey = (stat: Pick<AggregatedStat, 'provider_uuid' | 'model' | 'key'>) =>
    `${stat.provider_uuid || ''}::${stat.model || stat.key}`;

// provider_uuid is already the whole axis, so no collision-avoidance pairing
// is needed the way model+provider needs one.
export const getProviderKey = (stat: Pick<AggregatedStat, 'provider_uuid' | 'provider_name' | 'key'>) =>
    stat.provider_uuid || stat.provider_name || stat.key;

export const accountKey = (row: UserUsageRow) => row.user_id;
export const accountName = (row: UserUsageRow) => row.display_name;
export const accountSearchText = (row: UserUsageRow) => [row.display_name, row.user_id];
export const modelName = (row: AggregatedStat) => row.model || row.key;
export const modelSearchText = (row: AggregatedStat) => [row.model || row.key, row.provider_name || ''];
export const providerName = (row: AggregatedStat) => row.provider_name || row.key;
export const providerSearchText = (row: AggregatedStat) => [row.provider_name || row.key];

// The three "detail" loaders below are stable module-level functions (not
// closures), so useRosterAxis's effect dependency on `loadDetail` never
// changes identity across renders — no useCallback needed at the call site.
export async function fetchModelsForAccount(selected: UserUsageRow | undefined, range: TimeRange): Promise<AggregatedStat[]> {
    if (!selected) return [];
    const result = await api.getUsageStats({
        ...buildTimeParams(range),
        user_id: selected.user_id,
        group_by: 'model',
        sort_by: 'total_tokens',
        sort_order: 'desc',
        limit: 1000,
    });
    return result?.data || [];
}

// Which accounts used this model — scoped by provider + model together, not
// model name alone, since the same model name can exist under more than one
// provider.
export async function fetchAccountsForModel(selected: AggregatedStat | undefined, range: TimeRange): Promise<AggregatedStat[]> {
    if (!selected) return [];
    const result = await api.getUsageStats({
        ...buildTimeParams(range),
        model: selected.model,
        provider: selected.provider_uuid,
        group_by: 'user',
        sort_by: 'total_tokens',
        sort_order: 'desc',
        limit: 500,
    });
    return result?.data || [];
}

// Which accounts used this provider (across all its models).
export async function fetchAccountsForProvider(selected: AggregatedStat | undefined, range: TimeRange): Promise<AggregatedStat[]> {
    if (!selected) return [];
    const result = await api.getUsageStats({
        ...buildTimeParams(range),
        provider: selected.provider_uuid,
        group_by: 'user',
        sort_by: 'total_tokens',
        sort_order: 'desc',
        limit: 500,
    });
    return result?.data || [];
}
