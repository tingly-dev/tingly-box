import {Dialog, DialogContent} from '@mui/material';
import {render} from '@testing-library/react';
import {describe, expect, it, vi} from 'vitest';
import GuideDrawer from './GuideDrawer';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({t: (_key: string, o?: {defaultValue?: string}) => o?.defaultValue ?? _key}),
}));

describe('GuideDrawer', () => {
    it('stacks above a dialog it is opened from', () => {
        render(
            <Dialog open>
                <DialogContent>
                    <GuideDrawer open title="Guide" onClose={() => {}}>body</GuideDrawer>
                </DialogContent>
            </Dialog>,
        );
        const zIndex = (el: HTMLElement | null) => Number(getComputedStyle(el as HTMLElement).zIndex);
        const dialogRoot = document.querySelector('.MuiDialog-root') as HTMLElement;
        const drawerRoot = document.querySelector('.MuiDrawer-root') as HTMLElement;
        expect(zIndex(drawerRoot)).toBeGreaterThan(zIndex(dialogRoot));
    });
});
