import { renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useActivityItems } from './useActivityItems';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (_key: string, o?: { defaultValue?: string }) => o?.defaultValue ?? _key }),
}));
vi.mock('../contexts/FeatureFlagsContext', () => ({
    useFeatureFlags: () => ({ skillIde: false, enableGuardrails: false, enableBench: false, enableProjects: true }),
}));
vi.mock('./useProjectsWaiting', () => ({ useProjectsWaiting: () => false }));
vi.mock('@/contexts/ProfileContext', () => ({ useProfileContext: () => ({ profiles: {} }) }));
vi.mock('@/pages/mcp/profileStore', () => ({ useMCPProfiles: () => [] }));
vi.mock('@/pages/image/profiles/imageProfileStore', () => ({ useImageProfiles: () => [] }));
vi.mock('@/contexts/TeamContext', () => ({ useTeamContext: () => ({ teams: [] }) }));

describe('Workspace rail item', () => {
    const workspace = () => renderHook(() => useActivityItems()).result.current.find((i) => i.key === 'projects')!;

    it('opens the Workspace home, not the Projects list', () => {
        expect(workspace().defaultPath).toBe('/workspace');
    });

    it('stays lit on the home and on every page under it', () => {
        const child = workspace().children!.find((c) => c.type !== 'divider')!;
        const match = (child as { match: (p: string) => boolean }).match;
        for (const path of ['/workspace', '/projects', '/projects/p1', '/projects/folder', '/projects/im', '/bots/overview']) {
            expect(match(path), path).toBe(true);
        }
    });
});
