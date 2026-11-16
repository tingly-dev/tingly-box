import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { QuotaMenu } from './QuotaMenu';
import { SHELL_ROUTES } from '@/routes/shellRoutes';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string) => key }),
}));

const getProviders = vi.hoisted(() => vi.fn().mockResolvedValue({ data: [] }));
vi.mock('@/services/api', () => ({ api: { getProviders } }));
vi.mock('@/hooks/useProviderQuota', () => ({
    useProviderQuota: () => ({ quotaData: {}, refreshing: new Set(), loading: false }),
}));
vi.mock('@/utils/providerIcon', () => ({ useProviderIconId: () => () => undefined }));
vi.mock('@/components/ProviderLogo', () => ({ ProviderLabel: () => null }));
vi.mock('@/components/credential/QuotaCell', () => ({ QuotaCell: () => null }));

afterEach(() => {
    cleanup();
    vi.clearAllMocks();
});

function Location() {
    return <output data-testid="location">{useLocation().pathname}</output>;
}

it('loads the shared tray quota panel on demand and navigates to history in this window', async () => {
    const onNavigate = vi.fn();
    render(<MemoryRouter><QuotaMenu onNavigate={onNavigate} /><Location /></MemoryRouter>);
    const trigger = screen.getByRole('button', { name: 'Quota' });
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
    expect(getProviders).not.toHaveBeenCalled();

    fireEvent.click(trigger);
    expect(await screen.findByRole('dialog', { name: 'hub.quota.title' })).toBeVisible();
    expect(await screen.findByText('hub.quota.empty')).toBeVisible();
    expect(getProviders).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole('button', { name: 'layout.quotaHistory' }));
    expect(screen.getByTestId('location')).toHaveTextContent(SHELL_ROUTES.quotaHistory);
    expect(onNavigate).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
});

it('closes on Escape and restores focus to the ring', async () => {
    render(<MemoryRouter><QuotaMenu /></MemoryRouter>);
    const trigger = screen.getByRole('button', { name: 'Quota' });
    trigger.focus();
    fireEvent.click(trigger);
    const dialog = await screen.findByRole('dialog');
    await screen.findByText('hub.quota.empty');
    fireEvent.keyDown(dialog, { key: 'Escape' });
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(trigger).toHaveFocus();
});
