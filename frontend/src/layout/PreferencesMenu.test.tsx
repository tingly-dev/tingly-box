import { cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { MemoryRouter, useLocation } from 'react-router-dom';
import { PreferencesMenu } from './PreferencesMenu';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en', changeLanguage: vi.fn() } }),
}));
vi.mock('../contexts/ThemeContext', () => ({ useThemeMode: () => ({ mode: 'light', setTheme: vi.fn() }) }));
vi.mock('../contexts/VersionContext', () => ({ useVersion: () => ({ currentVersion: '1.0.0' }) }));
vi.mock('@/i18n', () => ({ SUPPORTED_LANGUAGES: [{ code: 'en', labelKey: 'lang.en' }], resolveLanguage: (l: string) => l }));
vi.mock('./PowerUpsMenu', () => ({ PowerUpsMenu: () => null }));

afterEach(cleanup);

function Location() {
    return <output data-testid="location">{useLocation().pathname}</output>;
}

it('opens the log page from the gear menu and closes the menu', () => {
    const onClose = vi.fn();
    render(
        <MemoryRouter initialEntries={['/dashboard']}>
            <PreferencesMenu anchorEl={document.body} onClose={onClose} />
            <Location />
        </MemoryRouter>,
    );
    fireEvent.click(screen.getByRole('menuitem', { name: 'layout.logs' }));
    expect(screen.getByTestId('location').textContent).toBe('/system/logs');
    expect(onClose).toHaveBeenCalled();
});
