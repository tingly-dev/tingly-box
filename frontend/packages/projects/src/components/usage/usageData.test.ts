import { describe, expect, it } from 'vitest';
import type { Usage } from '../../client';
import { usageEntries } from './usageData';

const usage = (by_model: Usage['by_model'], by_thread: Usage['by_thread']): Usage => ({ by_model, by_thread } as Usage);

describe('usageEntries', () => {
    it('ranks rows by tokens spent and gives each a share of its own card\'s total', () => {
        const { models, threads } = usageEntries(usage(
            [
                { model: 'small', threads: 2, input_tokens: 2_000_000, output_tokens: 1_000_000 },
                { model: 'cc[1m]', threads: 18, input_tokens: 300_000_000, output_tokens: 1_000_000 },
            ],
            [{ thread_id: 't1', title: 'Fix login', model: 'cc[1m]', turns: 5, input_tokens: 34_500_000, output_tokens: 98_500 }],
        ));
        expect(models[0]).toMatchObject({ key: 'cc[1m]', count: 18, share: 99 });
        expect(models[1]).toMatchObject({ key: 'small', share: 1 });
        expect(threads[0]).toMatchObject({ key: 't1', label: 'Fix login', secondary: 'cc[1m]', count: 5, share: 100 });
    });

    it('sums the strip over the thread rows — the threads are what reported the usage', () => {
        const { totals } = usageEntries(usage(
            [{ model: 'cc[1m]', threads: 2, input_tokens: 300, output_tokens: 100 }],
            [
                { thread_id: 't1', title: 'A', model: 'cc[1m]', turns: 3, input_tokens: 200, output_tokens: 60 },
                { thread_id: 't2', title: 'B', model: 'cc[1m]', turns: 4, input_tokens: 100, output_tokens: 40 },
            ],
        ));
        expect(totals).toEqual({ input: 300, output: 100, total: 400, threads: 2, turns: 7 });
    });

    it('keeps an unnamed model visible as —', () => {
        const { models } = usageEntries(usage([{ model: '', threads: 1, input_tokens: 10, output_tokens: 0 }], []));
        expect(models[0].label).toBe('—');
    });

    it('reads zero usage as zero shares, not NaN', () => {
        const { models, threads, totals } = usageEntries(usage([], []));
        expect(models).toEqual([]);
        expect(threads).toEqual([]);
        expect(totals).toEqual({ input: 0, output: 0, total: 0, threads: 0, turns: 0 });
    });
});
