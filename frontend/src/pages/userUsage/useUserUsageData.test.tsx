import { renderHook, waitFor, cleanup } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useUserUsageData } from './useUserUsageData';
import { accountKey, accountSearchText, fetchModelsForAccount } from './userUsageModel';
import api from '@/services/api';

const state = vi.hoisted(() => ({teams: [{id: 'a', name: 'Project A'}, {id: 'b', name: 'Project B'}]}));
vi.mock('@/contexts/TeamContext', () => ({useTeamContext: () => state}));
vi.mock('react-i18next', () => ({useTranslation: () => ({t: (_: string, options: {defaultValue: string}) => options.defaultValue})}));
vi.mock('@/components/dashboard', () => ({getTotalTokens: () => 0}));
vi.mock('@/services/api', () => ({default: {listAPITokens: vi.fn(), getUsageStats: vi.fn()}}));

afterEach(cleanup);
beforeEach(() => {
    vi.clearAllMocks();
    state.teams = [{id: 'a', name: 'Project A'}, {id: 'b', name: 'Project B'}];
    vi.mocked(api.listAPITokens).mockResolvedValue({success: true, data: {tokens: [
        {token_id: 'key-a', user_id: 'user-a', display_name: 'alex', team_id: 'a', enabled: true},
        {token_id: 'key-b', user_id: 'user-b', display_name: 'alex', team_id: 'b', enabled: true},
    ]}});
    vi.mocked(api.getUsageStats).mockImplementation(async (params) => ({data: params?.group_by === 'user' ? [
        {key: 'user-a', user_id: 'user-a', request_count: 11},
        {key: 'user-b', user_id: 'user-b', request_count: 22},
    ] : []}));
});

describe('Team-qualified usage accounts', () => {
    it('labels lists and drilldowns while preserving independent counts and user filters', async () => {
        const {result, rerender} = renderHook(() => useUserUsageData('7d'));
        await waitFor(() => expect(result.current.loading).toBe(false));
        const a = result.current.rows.find((r) => r.user_id === 'user-a')!;
        const b = result.current.rows.find((r) => r.user_id === 'user-b')!;
        expect(a.display_name).toBe('alex · Project A');
        expect(b.display_name).toBe('alex · Project B');
        expect(a.request_count).toBe(11); expect(b.request_count).toBe(22);
        expect(accountKey(a)).not.toBe(accountKey(b));
        expect(accountSearchText(b).join(' ')).toContain('Project B');
        expect(result.current.accountDisplayName('user-b')).toBe('alex · Project B');
        expect(result.current.accountDisplayName('admin')).toBe('Primary account');
        expect(result.current.accountDisplayName('deleted-user')).toBe('deleted-user');
        await fetchModelsForAccount(b, '7d');
        expect(api.getUsageStats).toHaveBeenLastCalledWith(expect.objectContaining({user_id: 'user-b', group_by: 'model'}));
        state.teams = [{id: 'a', name: 'new-project'}, {id: 'b', name: 'Project B'}];
        rerender();
        expect(result.current.accountDisplayName('user-a')).toBe('alex · new-project');
        expect(result.current.rows.find((r) => r.user_id === 'user-a')?.display_name).toBe('alex · new-project');
    });
});
