import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { AdvancedSection } from './AdvancedSection';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string) => key }),
}));

afterEach(cleanup);

it('starts collapsed even when settings are customised, and expands on click', () => {
    render(<AdvancedSection customizedCount={3}><span>inner</span></AdvancedSection>);
    const toggle = screen.getByRole('button', { name: /common.advanced/ });
    expect(toggle.getAttribute('aria-expanded')).toBe('false');
    expect(screen.getByText('common.advancedCustomized')).toBeTruthy();
    fireEvent.click(toggle);
    expect(toggle.getAttribute('aria-expanded')).toBe('true');
});
