// Shared types for TokenHistoryChart components

export interface TimeSeriesData {
    timestamp: string;
    request_count: number;
    total_tokens?: number;
    input_tokens: number;
    output_tokens: number;
    cache_read_tokens?: number;
    cache_write_tokens?: number;
    error_count?: number;
    avg_latency_ms?: number;
}

export interface ChartDataPoint {
    timestamp: string;
    time: string;
    timeFull: string;
    inputTokens: number;
    outputTokens: number;
    cacheTokens: number;
    // Cache writes are a SUBSET of inputTokens, never a fourth stacked series —
    // charting them separately would double count the same tokens.
    cacheWriteTokens: number;
    cacheRatio: number;
}

export type SeriesKey = 'cache' | 'input' | 'output';
export interface SeriesVisibility {
    cache: boolean;
    input: boolean;
    output: boolean;
}

export interface LegendItemProps {
    label: string;
    color: string;
    visible: boolean;
    onToggle: () => void;
}
