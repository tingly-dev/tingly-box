import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { UsageEntry } from './usageData';
import { UsageCard, UsageHeadCell, UsageRows, UsageStatStrip } from './UsageViews';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));
// formatTokens, the way every surface in this package renders a token count.
const formatTokens = (n: number): string => (n >= 1_000_000 ? `${(n / 1_000_000).toFixed(1).replace(/\.0$/, '')}M` : n >= 1_000 ? `${(n / 1_000).toFixed(1).replace(/\.0$/, '')}k` : String(n));

const entry = (o: Partial<UsageEntry> = {}): UsageEntry => ({
    key: 'cc[1m]', label: 'cc[1m]', count: 18, inputTokens: 305_000_000, outputTokens: 1_200_000, total: 306_200_000, share: 100, ...o,
});

afterEach(() => cleanup());

describe('UsageViews', () => {
    it('shows a tile per stat, label over value', () => {
        render(<UsageStatStrip stats={[{ label: 'usage.totalTokens', value: '306.2M' }, { label: 'usage.threads', value: '18' }]} />);
        expect(screen.getByText('usage.totalTokens')).toBeInTheDocument();
        expect(screen.getByText('306.2M')).toBeInTheDocument();
        expect(screen.getByText('18')).toBeInTheDocument();
    });

    it('renders a card as header (title, count chip) over the shared column order', () => {
        const rows = [entry(), entry({ key: 'small', label: 'small', secondary: 'cc[1m]', count: 5, inputTokens: 34_500_000, outputTokens: 98_500, total: 34_598_500, share: 11.3 })];
        render(
            <UsageCard title="usage.models" count={2} maxHeight={280}>
                <thead>
                    <tr>
                        <UsageHeadCell label="usage.model" />
                        <UsageHeadCell label="usage.threads" numeric />
                        <UsageHeadCell label="usage.share" numeric />
                        <UsageHeadCell label="usage.in" numeric />
                        <UsageHeadCell label="usage.out" numeric />
                    </tr>
                </thead>
                <UsageRows rows={rows} countTitle="usage.threads" formatTokens={formatTokens} />
            </UsageCard>,
        );
        expect(screen.getByText('usage.models')).toBeInTheDocument();
        expect(screen.getByText('2')).toBeInTheDocument(); // the count chip
        // In/out as compact strings, the exact figure only on hover.
        expect(screen.getByText('305M')).toBeInTheDocument();
        expect(screen.getByTitle('305000000')).toBeInTheDocument();
        // Share percent renders beside the meter; the bar itself is aria-hidden.
        expect(screen.getByText('11.3%')).toBeInTheDocument();
    });

    it('shows no count chip for an empty card', () => {
        render(
            <UsageCard title="usage.models" count={0}>
                <UsageRows rows={[]} countTitle="usage.threads" formatTokens={formatTokens} />
            </UsageCard>,
        );
        expect(screen.getByText('usage.models')).toBeInTheDocument();
        expect(screen.queryByText('0')).not.toBeInTheDocument();
    });
});
