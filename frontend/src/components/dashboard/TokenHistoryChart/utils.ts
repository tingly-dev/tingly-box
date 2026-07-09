// Shared utilities for TokenHistoryChart components

import type { ChartDataPoint, TimeSeriesData } from './types';
import { formatNumber } from '../chartStyles';

export const formatTimeLabel = (timestamp: string, isDayMode: boolean): string => {
    if (!timestamp) return '';

    let date: Date;
    const timestampNum = parseInt(timestamp, 10);

    if (!isNaN(timestampNum) && timestampNum > 1000000000 && timestampNum < 9999999999) {
        date = new Date(timestampNum * 1000);
    } else {
        date = new Date(timestamp);
    }

    if (isNaN(date.getTime())) {
        console.warn('Invalid timestamp:', timestamp);
        return '';
    }

    const pad = (n: number) => String(n).padStart(2, '0');

    if (isDayMode) {
        return `${pad(date.getMonth() + 1)}/${pad(date.getDate())}`;
    }

    return `${pad(date.getMonth() + 1)}/${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
};

export const formatTooltipTime = (timestamp: string, isDayMode: boolean): string => {
    if (!timestamp) return timestamp;

    let date: Date;
    const timestampNum = parseInt(timestamp, 10);

    if (!isNaN(timestampNum) && timestampNum > 1000000000 && timestampNum < 9999999999) {
        date = new Date(timestampNum * 1000);
    } else {
        date = new Date(timestamp);
    }

    if (isNaN(date.getTime())) {
        return timestamp;
    }

    const options: Intl.DateTimeFormatOptions = {
        month: 'short',
        day: 'numeric',
    };

    // Only show time in hour mode, not in day mode
    if (!isDayMode) {
        options.hour = '2-digit';
        options.minute = '2-digit';
    }

    return date.toLocaleDateString('en-US', options);
};

export const formatChartData = (data: TimeSeriesData[], isDayMode: boolean): ChartDataPoint[] => {
    return data.map((item) => {
        const cache = item.cache_input_tokens || 0;
        const input = item.input_tokens || 0;
        const cacheRatio = cache + input > 0 ? cache / (cache + input) : 0;
        return {
            timestamp: item.timestamp,
            time: formatTimeLabel(item.timestamp, isDayMode),
            timeFull: formatTooltipTime(item.timestamp, isDayMode),
            inputTokens: item.input_tokens,
            outputTokens: item.output_tokens,
            cacheTokens: cache,
            cacheRatio,
        };
    });
};

export const calculateLabelInterval = (dataLength: number): number => {
    if (dataLength <= 7) return 0;
    if (dataLength <= 14) return 1;
    if (dataLength <= 30) return 4;
    return Math.ceil(dataLength / 6);
};

export const formatYAxis = formatNumber;

export const formatTooltipValue = formatNumber;

/**
 * Aggregate minute-level time series data into 5-minute buckets.
 * Groups timestamps by rounding down to the nearest 5-minute boundary.
 */
export const aggregateTo5MinBuckets = (data: TimeSeriesData[]): TimeSeriesData[] => {
    if (!data.length) return [];

    const buckets = new Map<string, {
        timestamp: string;
        input_tokens: number;
        output_tokens: number;
        cache_input_tokens: number;
        request_count: number;
        error_count: number;
        total_latency_weight: number;
    }>();

    for (const item of data) {
        let date: Date;
        const timestampNum = parseInt(item.timestamp, 10);
        if (!isNaN(timestampNum) && timestampNum > 1000000000 && timestampNum < 9999999999) {
            date = new Date(timestampNum * 1000);
        } else {
            date = new Date(item.timestamp);
        }

        if (isNaN(date.getTime())) continue;

        // Round down to nearest 5-minute boundary
        const roundedMinutes = Math.floor(date.getMinutes() / 5) * 5;
        date.setMinutes(roundedMinutes, 0, 0);
        const bucketKey = date.toISOString();

        const existing = buckets.get(bucketKey);
        const requestCount = item.request_count || 0;
        const latency = item.avg_latency_ms || 0;

        if (existing) {
            existing.input_tokens += item.input_tokens || 0;
            existing.output_tokens += item.output_tokens || 0;
            existing.cache_input_tokens += item.cache_input_tokens || 0;
            existing.request_count += requestCount;
            existing.error_count += item.error_count || 0;
            existing.total_latency_weight += latency * requestCount;
        } else {
            buckets.set(bucketKey, {
                timestamp: bucketKey,
                input_tokens: item.input_tokens || 0,
                output_tokens: item.output_tokens || 0,
                cache_input_tokens: item.cache_input_tokens || 0,
                request_count: requestCount,
                error_count: item.error_count || 0,
                total_latency_weight: latency * requestCount,
            });
        }
    }

    // Convert buckets back to array, sorted by timestamp
    const result: TimeSeriesData[] = Array.from(buckets.values())
        .sort((a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime())
        .map((bucket) => ({
            timestamp: bucket.timestamp,
            input_tokens: bucket.input_tokens,
            output_tokens: bucket.output_tokens,
            cache_input_tokens: bucket.cache_input_tokens,
            request_count: bucket.request_count,
            error_count: bucket.error_count,
            avg_latency_ms: bucket.request_count > 0
                ? bucket.total_latency_weight / bucket.request_count
                : 0,
        }));

    return result;
};
