// Main TokenHistoryChart component - backward compatibility wrapper
// Delegates to appropriate chart based on data interval

import type { TimeSeriesData } from './TokenHistoryChart/types';
import { DailyTokenHistoryChart, HourlyTokenHistoryChart } from './TokenHistoryChart';

interface TokenHistoryChartProps {
    data: TimeSeriesData[];
    interval?: string;
}

/**
 * Token History Chart Component
 *
 * Displays token usage over time with different chart types:
 * - Daily view (multi-day): Bar chart
 * - Hourly view (single day): Area chart
 *
 * @param data - Time series data from the API
 * @param interval - Time interval ('day' or 'hour')
 */
export function TokenHistoryChart({ data, interval = 'day' }: TokenHistoryChartProps) {
    // Determine chart type based on interval or data characteristics
    const isHourlyMode = interval === 'hour' || (interval === 'day' && data.length <= 24);

    if (isHourlyMode) {
        return <HourlyTokenHistoryChart data={data} />;
    }

    return <DailyTokenHistoryChart data={data} />;
}

// Default export for backward compatibility
export default TokenHistoryChart;

// Re-export individual chart components for direct use if needed
export { DailyTokenHistoryChart } from './TokenHistoryChart/DailyChart';
export { HourlyTokenHistoryChart } from './TokenHistoryChart/HourlyChart';

// Re-export types
export type { TimeSeriesData } from './TokenHistoryChart/types';
