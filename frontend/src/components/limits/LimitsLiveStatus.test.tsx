import {render, screen} from '@testing-library/react';
import {describe, expect, it, vi} from 'vitest';
import LimitsLiveStatus from './LimitsLiveStatus';

// t() shows the interpolated numbers, so what the user would read is checkable.
vi.mock('react-i18next', () => ({
    useTranslation: () => ({
        t: (key: string, o?: Record<string, unknown>) =>
            key === 'limits.liveWindow' ? `${o?.requests} requests · ${o?.tokens} tokens in window` : key,
    }),
}));

describe('LimitsLiveStatus', () => {
    it('abbreviates the window usage and keeps the exact figures on hover', () => {
        render(<LimitsLiveStatus entry={{key: 'acct:p', state: 'ok', inflight: 1, requests: 1200, tokens: 1_700_000} as never}/>);
        const line = screen.getByText('1.2K requests · 1.7M tokens in window');
        expect(line.getAttribute('title')).toBe('1200 requests · 1700000 tokens in window');
    });

    it('stays quiet while nothing has been used', () => {
        render(<LimitsLiveStatus entry={{key: 'acct:p', state: 'ok', inflight: 0, requests: 0, tokens: 0} as never}/>);
        expect(screen.queryByText(/tokens in window/)).toBeNull();
    });
});
