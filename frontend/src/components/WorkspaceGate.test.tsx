import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, it, expect, vi } from 'vitest';
import ExperimentalFeatureGate from './ExperimentalFeatureGate';

const mocks = vi.hoisted(() => ({ set: vi.fn(), refresh: vi.fn() }));
vi.mock('@/contexts/FeatureFlagsContext', () => ({ useFeatureFlags: () => ({ loading: false, enableProjects: false, refresh: mocks.refresh }) }));
vi.mock('@/services/api', () => ({ api: { setScenarioFlag: mocks.set } }));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (_: string, options: { defaultValue: string }) => options.defaultValue }) }));

describe('Workspace gate', () => {
    it('asks before enabling, refreshes the feature state, and arms the Workspace home hint', async () => {
        localStorage.clear();
        mocks.set.mockResolvedValue({ success: true });
        render(<MemoryRouter><ExperimentalFeatureGate feature="projects"><div>Workspace content</div></ExperimentalFeatureGate></MemoryRouter>);
        expect(mocks.set).not.toHaveBeenCalled();
        expect(screen.queryByText('Workspace content')).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'Enable Workspace' }));
        await waitFor(() => expect(mocks.set).toHaveBeenCalledWith('_global', 'projects', true));
        await waitFor(() => expect(mocks.refresh).toHaveBeenCalled());
        // The enablement is what arms the Workspace home's first-visit IM hint.
        expect(localStorage.getItem('tingly-project.workspace.imHintArmed')).toBe('1');
    });
});
