// Shared utilities for TokenHistoryChart components

import type { ChartDataPoint, TimeSeriesData } from './types';

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

    return `${pad(date.getMonth() + 1)}/${pad(date.getDate())} ${pad(date.getHours())}:00`;
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
    return data.map((item) => ({
        timestamp: item.timestamp,
        time: formatTimeLabel(item.timestamp, isDayMode),
        timeFull: formatTooltipTime(item.timestamp, isDayMode),
        inputTokens: item.input_tokens,
        outputTokens: item.output_tokens,
        cacheTokens: item.cache_input_tokens || 0,
    }));
};

export const calculateLabelInterval = (dataLength: number): number => {
    if (dataLength <= 7) return 0;
    if (dataLength <= 14) return 1;
    if (dataLength <= 30) return 4;
    return Math.ceil(dataLength / 6);
};

export const formatYAxis = (value: number): string => {
    if (value >= 1000000) return `${(value / 1000000).toFixed(1)}M`;
    if (value >= 1000) return `${(value / 1000).toFixed(1)}K`;
    return value.toString();
};

export const formatTooltipValue = (value: number): string => {
    if (value >= 1000000) return `${(value / 1000000).toFixed(2)}M`;
    if (value >= 1000) return `${(value / 1000).toFixed(2)}K`;
    return value.toLocaleString();
};
