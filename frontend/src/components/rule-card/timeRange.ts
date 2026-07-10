export interface TimeRangeValue {
    start: string;
    end: string;
    timezone: string;
    outside: boolean;
}

export const DEFAULT_TIME_RANGE: TimeRangeValue = {
    start: '09:00',
    end: '17:00',
    timezone: 'UTC',
    outside: false,
};

const isTimeOfDay = (value: string): boolean => /^([01]\d|2[0-3]):[0-5]\d$/.test(value);

export const parseTimeRange = (value: string): TimeRangeValue | null => {
    try {
        const parsed: unknown = JSON.parse(value);
        if (!parsed || typeof parsed !== 'object') return null;
        const candidate = parsed as Partial<TimeRangeValue>;
        if (
            !isTimeOfDay(candidate.start ?? '') ||
            !isTimeOfDay(candidate.end ?? '') ||
            !candidate.timezone ||
            typeof candidate.outside !== 'boolean'
        ) {
            return null;
        }
        return {
            start: candidate.start!,
            end: candidate.end!,
            timezone: candidate.timezone!,
            outside: candidate.outside,
        };
    } catch {
        return null;
    }
};

export const serializeTimeRange = (value: TimeRangeValue): string => JSON.stringify(value);

export const isValidTimeRange = (value: TimeRangeValue | null): value is TimeRangeValue =>
    value !== null && isTimeOfDay(value.start) && isTimeOfDay(value.end) && value.start !== value.end && !!value.timezone;

export const formatTimeRange = (value: string): string | null => {
    const range = parseTimeRange(value);
    if (!range) return null;
    return `${range.outside ? 'Outside' : 'During'} ${range.start}–${range.end} · ${range.timezone}`;
};

export const timezoneOptions = (): string[] => {
    const supportedValuesOf = Intl.supportedValuesOf;
    if (typeof supportedValuesOf !== 'function') return ['UTC'];
    return ['UTC', ...supportedValuesOf('timeZone').filter((timezone) => timezone !== 'UTC')];
};
