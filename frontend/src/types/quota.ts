// Quota types for provider usage/limit information

export type WindowType = 'session' | 'daily' | 'weekly' | 'monthly' | 'custom' | 'balance';
export type UsageUnit = 'tokens' | 'requests' | 'credits' | 'currency' | 'percent';

export interface UsageWindow {
  type: WindowType;
  used: number;
  limit: number;
  used_percent: number;
  resets_at?: string;  // ISO timestamp
  window_minutes?: number;
  unit: UsageUnit;
  label: string;
  description?: string;
}

export interface UsageCost {
  used: number;
  limit: number;
  currency_code: string;
  resets_at?: string;
  label?: string;
}

export interface UsageAccount {
  id: string;
  name: string;
  email?: string;
  tier?: string;
  organization_id?: string;
}

export interface UsageBreakdown {
  key: string;           // 分组键，如模型名称 "gpt-4"
  label: string;         // 显示标签
  group: string;         // 分组类型，如 "model", "type"
  windows: UsageWindow[]; // 该分组的多维度配额窗口
  description?: string;
}

export interface ProviderQuota {
  provider_uuid: string;
  provider_name: string;
  provider_type: string;
  fetched_at: string;
  expires_at: string;
  primary?: UsageWindow;
  secondary?: UsageWindow;
  tertiary?: UsageWindow;
  cost?: UsageCost;
  account?: UsageAccount;
  breakdowns?: UsageBreakdown[];  // 分组明细
  last_error?: string;
  last_error_at?: string;
}

// Extended quota with breakdowns flattened for UI consumption
export interface QuotaDisplayItem {
  key: string;           // Unique identifier (e.g., model name or "primary")
  label: string;         // Display label
  group?: string;        // Group type ("model", "type", or undefined for aggregate)
  windows: {
    primary?: UsageWindow;
    secondary?: UsageWindow;
    tertiary?: UsageWindow;
  };
}

// Helper to convert ProviderQuota to display items
export function quotaToDisplayItems(quota: ProviderQuota): QuotaDisplayItem[] {
  const items: QuotaDisplayItem[] = [];

  // Add breakdowns first (per-model or per-type)
  if (quota.breakdowns && quota.breakdowns.length > 0) {
    for (const bd of quota.breakdowns) {
      // Find daily and weekly windows for this breakdown
      const daily = bd.windows.find(w => w.type === 'daily');
      const weekly = bd.windows.find(w => w.type === 'weekly');

      items.push({
        key: bd.key,
        label: bd.label,
        group: bd.group,
        windows: {
          primary: daily,
          secondary: weekly,
        },
      });
    }
  }

  // Add aggregate item at the end
  items.push({
    key: 'aggregate',
    label: 'Total',
    windows: {
      primary: quota.primary,
      secondary: quota.secondary,
      tertiary: quota.tertiary,
    },
  });

  return items;
}

// Extend ProviderModelData to include quota
export interface ProviderModelDataWithQuota {
  uuid: string;
  models: string[];
  star_models?: string[];
  custom_model?: string[];
  api_base?: string;
  last_updated?: string;
  quota?: ProviderQuota;
}
