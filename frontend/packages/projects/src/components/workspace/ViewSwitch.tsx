import { useState, type ReactNode } from 'react';
import { ToggleButton, ToggleButtonGroup, Tooltip } from '@mui/material';
import { useTranslation } from 'react-i18next';
import { readLocal, writeLocal } from '../../utils/storage';
import { Kanban, ViewList } from '../icons';

export type ThreadsView = 'list' | 'kanban';
const VIEW_KEY = 'overview.view';

/** How a project's threads are laid out in the Overview, remembered per browser; the list until chosen. */
export function useThreadsView(): [ThreadsView, (v: ThreadsView) => void] {
    const [view, setView] = useState<ThreadsView>(() => (readLocal(VIEW_KEY) === 'kanban' ? 'kanban' : 'list'));
    return [view, (v) => {
        writeLocal(VIEW_KEY, v);
        setView(v);
    }];
}

/** The List | Kanban switch. */
export function ViewSwitch({ view, onChange }: { view: ThreadsView; onChange: (v: ThreadsView) => void }) {
    const { t } = useTranslation('projects');
    const button = (value: ThreadsView, label: string, icon: ReactNode) => (
        <Tooltip title={label}>
            <ToggleButton value={value} aria-label={label} sx={{ p: 0.5, border: 0, borderRadius: 1, color: 'text.disabled', '& svg': { fontSize: 18 }, '&.Mui-selected': { color: 'text.primary', bgcolor: 'action.selected' } }}>
                {icon}
            </ToggleButton>
        </Tooltip>
    );
    return (
        <ToggleButtonGroup exclusive size="small" value={view} aria-label={t('overview.view.label')} onChange={(_, v: ThreadsView | null) => v && onChange(v)} sx={{ gap: 0.25 }}>
            {button('list', t('overview.view.list'), <ViewList />)}
            {button('kanban', t('overview.view.kanban'), <Kanban />)}
        </ToggleButtonGroup>
    );
}
