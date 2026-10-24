import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import type { ProviderQuota } from '@/types/quota';
import { QuotaCell } from './QuotaCell';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string) => key }),
}));

const notify = vi.hoisted(() => ({ success: vi.fn(), warning: vi.fn(), error: vi.fn() }));
vi.mock('@/hooks/useNotify', () => ({ useNotify: () => notify }));

afterEach(() => {
    cleanup();
    Object.values(notify).forEach(fn => fn.mockClear());
});

const quota = {
    provider_uuid: 'glm',
    provider_name: 'GLM',
    provider_type: 'glm',
    fetched_at: new Date().toISOString(),
    expires_at: new Date().toISOString(),
    windows: [],
    breakdowns: [{
        key: 'reset-card-101',
        label: 'Weekly reset card',
        group: 'resource',
        windows: [{ type: 'balance', used: 0, limit: 1, used_percent: 0, unit: 'credits', label: 'available', description: 'Available' }],
        redeem: { id: '101', scope: 'weekly' },
    }],
} as unknown as ProviderQuota;

function hover() {
    fireEvent.mouseOver(screen.getByRole('button'));
}

describe('QuotaCell reset cards', () => {
    it('offers no spend action without a handler', async () => {
        render(<QuotaCell quota={quota} refreshing={false} onRefresh={vi.fn()} />);
        hover();
        await screen.findByText('Weekly reset card');
        expect(screen.queryByText('providerTable.quota.redeem.use')).toBeNull();
    });

    it('asks before spending, then spends exactly the listed card once', async () => {
        const onRedeem = vi.fn().mockResolvedValue({ status: 'spent' });
        render(<QuotaCell quota={quota} refreshing={false} onRefresh={vi.fn()} onRedeem={onRedeem} />);
        hover();
        fireEvent.click(await screen.findByText('providerTable.quota.redeem.use'));

        // Nothing is sent until the dialog is confirmed.
        expect(onRedeem).not.toHaveBeenCalled();
        expect(screen.getByRole('dialog')).toHaveTextContent('providerTable.quota.redeem.weekly');

        fireEvent.click(screen.getByRole('button', { name: 'providerTable.quota.redeem.confirm' }));
        await waitFor(() => expect(onRedeem).toHaveBeenCalledWith('101'));
        expect(onRedeem).toHaveBeenCalledTimes(1);
        await waitFor(() => expect(notify.success).toHaveBeenCalledWith('providerTable.quota.redeem.spent'));
        await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    });

    it('reports an unconfirmed spend as a warning, not a success', async () => {
        const onRedeem = vi.fn().mockResolvedValue({ status: 'unconfirmed' });
        render(<QuotaCell quota={quota} refreshing={false} onRefresh={vi.fn()} onRedeem={onRedeem} />);
        hover();
        fireEvent.click(await screen.findByText('providerTable.quota.redeem.use'));
        fireEvent.click(screen.getByRole('button', { name: 'providerTable.quota.redeem.confirm' }));
        await waitFor(() => expect(notify.warning).toHaveBeenCalledWith('providerTable.quota.redeem.unconfirmed'));
    });

    it('checks in without a dialog', async () => {
        const checkin = {
            ...quota,
            breakdowns: [{ ...quota.breakdowns![0], key: 'daily-checkin', label: 'Daily check-in', redeem: { id: 'daily-checkin', scope: 'daily', auto: true } }],
        } as unknown as ProviderQuota;
        const onRedeem = vi.fn().mockResolvedValue({ status: 'spent' });
        render(<QuotaCell quota={checkin} refreshing={false} onRefresh={vi.fn()} onRedeem={onRedeem} />);
        hover();
        fireEvent.click(await screen.findByText('providerTable.quota.redeem.checkin'));
        await waitFor(() => expect(onRedeem).toHaveBeenCalledWith('daily-checkin'));
        expect(screen.queryByRole('dialog')).toBeNull();
        await waitFor(() => expect(notify.success).toHaveBeenCalledWith('providerTable.quota.redeem.checkedIn'));
    });
});
