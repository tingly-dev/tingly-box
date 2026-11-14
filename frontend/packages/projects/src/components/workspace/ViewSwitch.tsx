import { useState, type ReactNode } from 'react';
import { ToggleButton, ToggleButtonGroup, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { readLocal, writeLocal } from '../../utils/storage';
import { Kanban, ViewList } from '../icons';
import { radius } from '@tingly/ui/theme/radius';

/** A list, or a board with a lane per status. */
export type ThreadsView = 'list' | 'board';
const VIEW_KEY = 'overview.view';

function readView(): ThreadsView {
    const v = readLocal(VIEW_KEY);
    // The board once had two orientations ('vertical' and 'horizontal', and 'kanban' before them); it is one now.
    return v === 'board' || v === 'vertical' || v === 'horizontal' || v === 'kanban' ? 'board' : 'list';
}

/** How a project's threads are laid out in the Overview, remembered per browser; the list until chosen. */
export function useThreadsView(): [ThreadsView, (v: ThreadsView) => void] {
    const [view, setView] = useState<ThreadsView>(readView);
    return [view, (v) => {
        writeLocal(VIEW_KEY, v);
        setView(v);
    }];
}

/** The List | Board switch. */
export function ViewSwitch({ view, onChange }: { view: ThreadsView; onChange: (v: ThreadsView) => void }) {
    const { t } = useTranslation('projects');
    const button = (value: ThreadsView, label: string, icon: ReactNode) => (
        <Tooltip title={label}>
            <ToggleButton value={value} aria-label={label} sx={{ p: 0.5, border: 0, borderRadius: radius.row, color: 'text.disabled', '& svg': { fontSize: 18 }, '&.Mui-selected': { color: 'text.primary', bgcolor: 'action.selected' } }}>
                {icon}
            </ToggleButton>
        </Tooltip>
    );
    return (
        <ToggleButtonGroup exclusive size="small" value={view} aria-label={t('overview.view.label')} onChange={(_, v: ThreadsView | null) => v && onChange(v)} sx={{ gap: 0.25 }}>
            {button('list', t('overview.view.list'), <ViewList />)}
            {button('board', t('overview.view.board'), <Kanban />)}
        </ToggleButtonGroup>
    );
}
