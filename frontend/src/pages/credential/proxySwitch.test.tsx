import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useState } from 'react';
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '@/i18n';
import { api } from '@/services/api';
import { useProviderProxy } from '@/pages/credential/useProviderProxy';
import { CredentialCards } from '../../components/credential/CredentialCards';
import { ProxySwitch } from '../../components/credential/ProxySwitch';

vi.mock('@/services/api', () => ({ api: { updateProvider: vi.fn(), getConfig: vi.fn() } }));
vi.mock('../../components/credential/QuotaCell', () => ({ QuotaCell: () => null }));

beforeAll(async () => { await i18n.changeLanguage('en'); });
beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(api.getConfig).mockResolvedValue({ data: { http_transport: { global_proxy_url: '' } } });
});
afterEach(cleanup);

const withProxy = { uuid: 'p1', name: 'Alpha', enabled: true, api_style: 'openai', api_base: 'https://a.example/v1', proxy_url: 'http://127.0.0.1:7890' };
const noProxy = { ...withProxy, uuid: 'p2', name: 'Beta', proxy_url: '' };

describe('ProxySwitch', () => {
    it('is a switch that is on when a proxy URL is stored', () => {
        const proxy = { pending: new Set<string>(), onDisable: vi.fn(), onEnable: vi.fn() };
        render(<ProxySwitch provider={withProxy} proxy={proxy} />);
        const control = screen.getByRole('switch', { name: 'Proxy for Alpha' });
        expect(control).toHaveAttribute('aria-checked', 'true');
        fireEvent.click(control);
        expect(proxy.onDisable).toHaveBeenCalledWith('p1');
        expect(proxy.onEnable).not.toHaveBeenCalled();
    });
    it('is off with none stored and asks to enable', () => {
        const proxy = { pending: new Set<string>(), onDisable: vi.fn(), onEnable: vi.fn() };
        render(<ProxySwitch provider={noProxy} proxy={proxy} />);
        const control = screen.getByRole('switch', { name: 'Proxy for Beta' });
        expect(control).toHaveAttribute('aria-checked', 'false');
        fireEvent.click(control);
        expect(proxy.onEnable).toHaveBeenCalledWith('p2');
    });
    it('is inert while saving and absent without a handler set', () => {
        const { rerender } = render(<ProxySwitch provider={withProxy} proxy={{ pending: new Set(['p1']), onDisable: vi.fn(), onEnable: vi.fn() }} />);
        expect(screen.getByRole('switch')).toBeDisabled();
        rerender(<ProxySwitch provider={withProxy} />);
        expect(screen.queryByRole('switch')).toBeNull();
    });
});

function Harness({ notify }: { notify: (m: string, s: 'success' | 'error') => void }) {
    const [providers, setProviders] = useState<any[]>([withProxy, noProxy]);
    const { proxyControl, proxyDialog } = useProviderProxy(providers, { setProviders, notify });
    return <><CredentialCards layout="list" providers={providers} onModels={() => {}} onMore={() => {}} proxy={proxyControl} />{proxyDialog}</>;
}
const stateOf = (name: string) => screen.getByRole('switch', { name: `Proxy for ${name}`, hidden: true }).getAttribute('aria-checked');

describe('credential proxy off/on', () => {
    it('off clears proxy_url through the ordinary update, at once', async () => {
        let resolve!: (v: unknown) => void;
        vi.mocked(api.updateProvider).mockReturnValue(new Promise((r) => { resolve = r; }));
        const notify = vi.fn();
        render(<Harness notify={notify} />);
        fireEvent.click(screen.getByRole('switch', { name: 'Proxy for Alpha' }));
        expect(stateOf('Alpha')).toBe('false');
        expect(screen.getByRole('switch', { name: 'Proxy for Alpha' })).toBeDisabled();
        resolve({ success: true });
        await waitFor(() => expect(notify).toHaveBeenCalledWith('Proxy off for Alpha', 'success'));
        expect(api.updateProvider).toHaveBeenCalledTimes(1);
        expect(api.updateProvider).toHaveBeenCalledWith('p1', { proxy_url: '' });
        expect(stateOf('Alpha')).toBe('false');
    });
    it('rolls back the old URL and says why when the save fails', async () => {
        vi.mocked(api.updateProvider).mockResolvedValue({ success: false, error: 'boom' });
        const notify = vi.fn();
        render(<Harness notify={notify} />);
        fireEvent.click(screen.getByRole('switch', { name: 'Proxy for Alpha' }));
        await waitFor(() => expect(notify).toHaveBeenCalled());
        expect(stateOf('Alpha')).toBe('true');
        expect(notify).toHaveBeenCalledWith("Couldn't change the proxy for Alpha: boom", 'error');
    });
    it('on asks for a URL, prefilled with the global proxy, and stores it on confirm', async () => {
        vi.mocked(api.getConfig).mockResolvedValue({ data: { http_transport: { global_proxy_url: 'http://10.0.0.1:3128' } } });
        vi.mocked(api.updateProvider).mockResolvedValue({ success: true });
        const notify = vi.fn();
        render(<Harness notify={notify} />);
        fireEvent.click(screen.getByRole('switch', { name: 'Proxy for Beta' }));
        const field = await screen.findByLabelText('HTTP/SOCKS Proxy URL (Optional)');
        await waitFor(() => expect(field).toHaveValue('http://10.0.0.1:3128'));
        expect(screen.getByText('Prefilled with the quick proxy from System Settings.')).toBeVisible();
        expect(api.updateProvider).not.toHaveBeenCalled();
        fireEvent.click(screen.getByRole('button', { name: 'Turn on' }));
        await waitFor(() => expect(api.updateProvider).toHaveBeenCalledWith('p2', { proxy_url: 'http://10.0.0.1:3128' }));
        await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
        expect(stateOf('Beta')).toBe('true');
    });
    it('on without a global proxy needs a valid URL and cancelling changes nothing', async () => {
        render(<Harness notify={vi.fn()} />);
        fireEvent.click(screen.getByRole('switch', { name: 'Proxy for Beta' }));
        const field = await screen.findByLabelText('HTTP/SOCKS Proxy URL (Optional)');
        expect(field).toHaveValue('');
        fireEvent.click(screen.getByRole('button', { name: 'Turn on' }));
        expect(screen.getByText(/Enter a URL like http:\/\/host:port/)).toBeVisible();
        fireEvent.change(field, { target: { value: 'ftp://nope' } });
        fireEvent.click(screen.getByRole('button', { name: 'Turn on' }));
        expect(api.updateProvider).not.toHaveBeenCalled();
        fireEvent.click(screen.getByRole('button', { name: 'Cancel' }));
        await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
        expect(stateOf('Beta')).toBe('false');
    });
    it('keeps the dialog open and the row off when enabling fails', async () => {
        vi.mocked(api.updateProvider).mockResolvedValue({ success: false, error: 'nope' });
        const notify = vi.fn();
        render(<Harness notify={notify} />);
        fireEvent.click(screen.getByRole('switch', { name: 'Proxy for Beta' }));
        fireEvent.change(await screen.findByLabelText('HTTP/SOCKS Proxy URL (Optional)'), { target: { value: 'socks5://127.0.0.1:1080' } });
        fireEvent.click(screen.getByRole('button', { name: 'Turn on' }));
        await waitFor(() => expect(notify).toHaveBeenCalledWith("Couldn't change the proxy for Beta: nope", 'error'));
        expect(screen.getByRole('dialog')).toBeInTheDocument();
        expect(stateOf('Beta')).toBe('false');
    });
});
