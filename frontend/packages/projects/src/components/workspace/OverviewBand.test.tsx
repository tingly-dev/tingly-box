import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { Thread } from '../../client';
import { BAND_DEFAULT } from '../project/PaneResizer';
import { OverviewBand } from './OverviewBand';

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }) }));
afterEach(() => {
    cleanup();
    localStorage.clear();
});

const KEY = 'tingly-project.overviewBandHeight';
const threads = [
    { id: 'a', title: 'Waiting one', status: 'waiting', updated_at: '2026-10-22T00:00:00Z' },
    { id: 'b', title: 'Running one', status: 'working', updated_at: '2026-10-21T00:00:00Z' },
] as Thread[];

const show = () => render(<OverviewBand threads={threads} open onToggle={() => {}} onOpen={() => {}} onLibrary={() => {}} onStatus={() => {}} />);
const handle = () => screen.getByRole('separator', { name: 'project.resizeBand' });

describe('OverviewBand buttons', () => {
    it('opens the Library and the Status', () => {
        const onLibrary = vi.fn();
        const onStatus = vi.fn();
        render(<OverviewBand threads={threads} open onToggle={() => {}} onOpen={() => {}} onLibrary={onLibrary} onStatus={onStatus} />);
        fireEvent.click(screen.getByRole('button', { name: 'overview.library' }));
        fireEvent.click(screen.getByRole('button', { name: 'overview.status' }));
        expect(onLibrary).toHaveBeenCalledTimes(1);
        expect(onStatus).toHaveBeenCalledTimes(1);
    });
});

describe('OverviewBand resizing', () => {
    it('fits its content until resized, then keeps the height per browser', () => {
        show();
        expect(handle()).toHaveAttribute('aria-orientation', 'horizontal');
        expect(handle()).toHaveAttribute('aria-valuenow', String(BAND_DEFAULT));
        expect(localStorage.getItem(KEY)).toBeNull();

        fireEvent.keyDown(handle(), { key: 'ArrowDown' });
        expect(localStorage.getItem(KEY)).toBe(String(BAND_DEFAULT + 16));
        fireEvent.keyDown(handle(), { key: 'ArrowDown', shiftKey: true });
        expect(handle()).toHaveAttribute('aria-valuenow', String(BAND_DEFAULT + 16 + 64));

        cleanup();
        show();
        expect(handle()).toHaveAttribute('aria-valuenow', String(BAND_DEFAULT + 16 + 64));
    });

    it('clamps to the smallest and largest height', () => {
        localStorage.setItem(KEY, '5000');
        show();
        const max = Number(handle().getAttribute('aria-valuenow'));
        expect(max).toBeLessThanOrEqual(480);
        expect(max).toBeGreaterThan(BAND_DEFAULT);
        fireEvent.keyDown(handle(), { key: 'ArrowDown' });
        expect(handle()).toHaveAttribute('aria-valuenow', String(max));
        for (let i = 0; i < 40; i++) fireEvent.keyDown(handle(), { key: 'ArrowUp', shiftKey: true });
        expect(handle()).toHaveAttribute('aria-valuenow', String(96));
    });

    it('goes back to fitting its content on double-click', () => {
        localStorage.setItem(KEY, '300');
        show();
        expect(handle()).toHaveAttribute('aria-valuenow', '300');
        fireEvent.doubleClick(handle());
        expect(localStorage.getItem(KEY)).toBeNull();
        expect(handle()).toHaveAttribute('aria-valuenow', String(BAND_DEFAULT));
    });

    it('has no handle while closed', () => {
        render(<OverviewBand threads={threads} open={false} onToggle={() => {}} onOpen={() => {}} onLibrary={() => {}} onStatus={() => {}} />);
        expect(screen.queryByRole('separator')).not.toBeInTheDocument();
    });
});
