import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '@/i18n';
import { api } from '@/services/api';
import { useProviderProxySwitch } from '@/hooks/useProviderProxySwitch';
import { CredentialCards } from './CredentialCards';
import { ProxySwitch } from './ProxySwitch';
import { effectiveProxyUrl, proxyState, redactProxyUrl } from './proxyState';

vi.mock('@/services/api', () => ({ api: { setProviderProxy: vi.fn() } }));
vi.mock('./QuotaCell', () => ({ QuotaCell: () => null }));

beforeAll(async () => { await i18n.changeLanguage('en'); });
beforeEach(() => { vi.clearAllMocks(); });
afterEach(cleanup);

const base = { uuid: 'p1', name: 'Alpha', enabled: true, api_style: 'openai', api_base: 'https://a.example/v1', proxy_url: 'http://127.0.0.1:7890' };
const second = { ...base, uuid: 'p2', name: 'Beta', proxy_url: '' };

describe('proxy state helpers', () => {
    it('derives none/on/off and the effective URL', () => {
        expect(proxyState({ proxy_url: '' })).toBe('none');
        expect(proxyState({ proxy_url: 'http://h:1' })).toBe('on');
        expect(proxyState({ proxy_url: 'http://h:1', proxy_disabled: true })).toBe('off');
        expect(effectiveProxyUrl({ proxy_url: 'http://h:1' })).toBe('http://h:1');
        expect(effectiveProxyUrl({ proxy_url: 'http://h:1', proxy_disabled: true })).toBe('');
    });
    it('never exposes proxy credentials', () => {
        expect(redactProxyUrl('http://user:pw@h:1')).toBe('http://***@h:1');
        expect(redactProxyUrl('socks5://h:1080')).toBe('socks5://h:1080');
    });
});

describe('ProxySwitch', () => {
    it('is a labelled switch carrying the state', () => {
        const onToggle = vi.fn();
        const { rerender } = render(<ProxySwitch provider={base} onToggle={onToggle} />);
        const control = screen.getByRole('switch', { name: 'Proxy for Alpha' });
        expect(control).toHaveAttribute('aria-checked', 'true');
        fireEvent.click(control);
        expect(onToggle).toHaveBeenCalledWith('p1');
        rerender(<ProxySwitch provider={{ ...base, proxy_disabled: true }} onToggle={onToggle} />);
        expect(screen.getByRole('switch', { name: 'Proxy for Alpha' })).toHaveAttribute('aria-checked', 'false');
    });
    it('is inert while a request is in flight', () => {
        const onToggle = vi.fn();
        render(<ProxySwitch provider={base} pending onToggle={onToggle} />);
        const control = screen.getByRole('switch');
        expect(control).toBeDisabled();
        expect(control).toHaveAttribute('aria-busy', 'true');
    });
    it('leads to the editor when no proxy is stored', () => {
        const onConfigure = vi.fn();
        render(<ProxySwitch provider={second} onConfigure={onConfigure} />);
        expect(screen.queryByRole('switch')).toBeNull();
        fireEvent.click(screen.getByRole('button', { name: 'Add a proxy for Beta' }));
        expect(onConfigure).toHaveBeenCalledWith('p2');
    });
});

function Harness({ notify }: { notify: (m: string, s: 'success' | 'error') => void }) {
    const [providers, setProviders] = useState<any[]>([base, second]);
    const { toggleProxy, proxyPending } = useProviderProxySwitch(providers, { setProviders, notify });
    return <CredentialCards layout="list" providers={providers} onModels={() => {}} onMore={() => {}} onToggleProxy={toggleProxy} proxyPending={proxyPending} />;
}
const stateOf = (name: string) => screen.getByRole('switch', { name: `Proxy for ${name}` }).getAttribute('aria-checked');

describe('credential list proxy switch', () => {
    it('flips at once, then keeps the server answer and says so', async () => {
        let resolve!: (v: unknown) => void;
        vi.mocked(api.setProviderProxy).mockReturnValue(new Promise((r) => { resolve = r; }));
        const notify = vi.fn();
        render(<Harness notify={notify} />);
        fireEvent.click(screen.getByRole('switch', { name: 'Proxy for Alpha' }));
        expect(stateOf('Alpha')).toBe('false');
        expect(screen.getByRole('switch', { name: 'Proxy for Alpha' })).toBeDisabled();
        fireEvent.click(screen.getByRole('switch', { name: 'Proxy for Alpha' }));
        await act(async () => { resolve({ success: true, data: { proxy_url: base.proxy_url, proxy_disabled: true } }); });
        await waitFor(() => expect(screen.getByRole('switch', { name: 'Proxy for Alpha' })).not.toBeDisabled());
        expect(api.setProviderProxy).toHaveBeenCalledTimes(1);
        expect(api.setProviderProxy).toHaveBeenCalledWith('p1', false);
        expect(stateOf('Alpha')).toBe('false');
        expect(notify).toHaveBeenCalledWith('Proxy off for Alpha', 'success');
    });
    it('turns it back on using the kept URL', async () => {
        vi.mocked(api.setProviderProxy).mockResolvedValueOnce({ success: true, data: { proxy_url: base.proxy_url, proxy_disabled: true } })
            .mockResolvedValueOnce({ success: true, data: { proxy_url: base.proxy_url, proxy_disabled: false } });
        render(<Harness notify={vi.fn()} />);
        fireEvent.click(screen.getByRole('switch', { name: 'Proxy for Alpha' }));
        await waitFor(() => expect(screen.getByRole('switch', { name: 'Proxy for Alpha' })).not.toBeDisabled());
        fireEvent.click(screen.getByRole('switch', { name: 'Proxy for Alpha' }));
        await waitFor(() => expect(api.setProviderProxy).toHaveBeenLastCalledWith('p1', true));
        await waitFor(() => expect(stateOf('Alpha')).toBe('true'));
    });
    it('rolls back and reports a failed request', async () => {
        vi.mocked(api.setProviderProxy).mockResolvedValue({ success: false, error: 'boom' });
        const notify = vi.fn();
        render(<Harness notify={notify} />);
        fireEvent.click(screen.getByRole('switch', { name: 'Proxy for Alpha' }));
        await waitFor(() => expect(notify).toHaveBeenCalled());
        expect(stateOf('Alpha')).toBe('true');
        expect(notify).toHaveBeenCalledWith("Couldn't change the proxy for Alpha: boom", 'error');
        expect(screen.getByRole('switch', { name: 'Proxy for Alpha' })).not.toBeDisabled();
    });
    it('rolls back when the request throws', async () => {
        vi.mocked(api.setProviderProxy).mockRejectedValue(new Error('offline'));
        const notify = vi.fn();
        render(<Harness notify={notify} />);
        fireEvent.click(screen.getByRole('switch', { name: 'Proxy for Alpha' }));
        await waitFor(() => expect(notify).toHaveBeenCalled());
        expect(stateOf('Alpha')).toBe('true');
    });
});
