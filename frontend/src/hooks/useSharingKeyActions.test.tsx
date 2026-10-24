import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useSharingKeyActions } from './useSharingKeyActions';

const mocks = vi.hoisted(() => ({ save: vi.fn(), changed: vi.fn(), error: vi.fn(), providers: vi.fn() }));
vi.mock('@/services/api', () => ({ api: {setAPITokenQuota: mocks.save, getProviders: mocks.providers} }));
vi.mock('@/hooks/useNotify', () => ({ useNotify: () => ({error: mocks.error}) }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({t: (key: string) => key}) }));

function Editor({mode = 'legacy'}: {mode?: 'provider' | 'legacy'} = {}) {
    const actions = useSharingKeyActions({teams: [], onChanged: mocks.changed});
    return <>
        <button onClick={() => actions.tableProps.onQuota?.({
            token_id: 'tb-share-test', user_id: 'user', team_id: 'team', display_name: 'CI',
            enabled: true, created_at: '2026-10-01', weekly_token_limit: 100, weekly_tokens_used: 40,
            quota_reset: {mode},
        })}>Edit quota</button>
        {actions.dialogs}
    </>;
}

describe('Sharing Key quota editor', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        mocks.save.mockResolvedValue({success: true});
        mocks.providers.mockResolvedValue({success: true, data: []});
    });

    it.each([['', null], ['0', 0], ['200', 200]] as const)('saves %s without resetting usage', async (value, expected) => {
        render(<Editor />);
        fireEvent.click(screen.getByText('Edit quota'));
        fireEvent.change(screen.getByRole('spinbutton'), {target: {value}});
        await act(async () => { fireEvent.click(screen.getByText('common.save')); });
        expect(mocks.save).toHaveBeenCalledWith('tb-share-test', expected, {mode: 'legacy'});
        expect(mocks.changed).toHaveBeenCalledOnce();
    });

    it.each(['-1', '1.5', '9007199254740992'])('rejects invalid limit %s', (value) => {
        render(<Editor />);
        fireEvent.click(screen.getByText('Edit quota'));
        fireEvent.change(screen.getByRole('spinbutton'), {target: {value}});
        expect(screen.getByText('common.save')).toBeDisabled();
        expect(mocks.save).not.toHaveBeenCalled();
    });

    it('keeps the dialog open when saving fails', async () => {
        mocks.save.mockResolvedValue({success: false, error: 'storage unavailable'});
        render(<Editor />);
        fireEvent.click(screen.getByText('Edit quota'));
        fireEvent.click(screen.getByText('common.save'));
        await waitFor(() => expect(mocks.error).toHaveBeenCalledWith('storage unavailable'));
        expect(screen.getByRole('spinbutton')).toBeInTheDocument();
        expect(mocks.changed).not.toHaveBeenCalled();
    });

    it('defaults to provider mode and selects the single real provider', async () => {
        mocks.providers.mockResolvedValue({success: true, data: [
            {uuid: 'builtin', name: 'Builtin', source: 'builtin'},
            {uuid: 'openai', name: 'My subscription', enabled: true},
        ]});
        render(<Editor mode="provider" />);
        fireEvent.click(screen.getByText('Edit quota'));
        await waitFor(() => expect(screen.getByRole('combobox', {name: 'sharingKeys.quota.provider'})).toHaveTextContent('My subscription'));
        expect(screen.getByText('sharingKeys.quota.unknownReset')).toBeInTheDocument();
        await act(async () => { fireEvent.click(screen.getByText('common.save')); });
        expect(mocks.save).toHaveBeenCalledWith('tb-share-test', 100, {mode: 'provider', provider_id: 'openai'});
    });

    it('validates a manual anchor and sends a UTC instant', async () => {
        render(<Editor mode="provider" />);
        fireEvent.click(screen.getByText('Edit quota'));
        fireEvent.mouseDown(screen.getByRole('combobox', {name: 'sharingKeys.quota.resetMode'}));
        fireEvent.click(await screen.findByRole('option', {name: 'sharingKeys.quota.manualReset'}));
        expect(screen.getByText('common.save')).toBeDisabled();
        fireEvent.change(screen.getByLabelText('sharingKeys.quota.anchor'), {target: {value: '2026-11-01T08:30'}});
        await act(async () => { fireEvent.click(screen.getByText('common.save')); });
        expect(mocks.save).toHaveBeenCalledWith('tb-share-test', 100, {mode: 'manual', anchor: new Date('2026-11-01T08:30').toISOString()});
    });

    it('requires a choice when multiple providers exist', async () => {
        mocks.providers.mockResolvedValue({success: true, data: [
            {uuid: 'a', name: 'Account A'}, {uuid: 'b', name: 'Account B'},
        ]});
        render(<Editor mode="provider" />);
        fireEvent.click(screen.getByText('Edit quota'));
        await waitFor(() => expect(screen.getByText('common.save')).toBeDisabled());
        fireEvent.mouseDown(screen.getByRole('combobox', {name: 'sharingKeys.quota.provider'}));
        fireEvent.click(await screen.findByRole('option', {name: 'Account B'}));
        await act(async () => { fireEvent.click(screen.getByText('common.save')); });
        expect(mocks.save).toHaveBeenCalledWith('tb-share-test', 100, {mode: 'provider', provider_id: 'b'});
    });
});
