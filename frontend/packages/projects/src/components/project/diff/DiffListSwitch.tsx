import { useState } from 'react';
import { ToggleButton, ToggleButtonGroup, Tooltip } from '@mui/material';
import { radius } from '@tingly/ui/theme/radius';
import { useTranslation } from 'react-i18next';
import { readLocal, writeLocal } from '../../../utils/storage';
import { ViewList, ViewTree } from '../../icons';

/** The changed files as a folder tree, or as one flat list. */
export type DiffListMode = 'tree' | 'list';
const MODE_KEY = 'diff.listMode';

/** Remembered per browser; the tree until chosen (a flat list of hundreds of
 * files is a wall of base names, and a small change is nearly flat as a tree anyway). */
export function useDiffListMode(): [DiffListMode, (m: DiffListMode) => void] {
    const [mode, setMode] = useState<DiffListMode>(() => (readLocal(MODE_KEY) === 'list' ? 'list' : 'tree'));
    return [mode, (m) => {
        writeLocal(MODE_KEY, m);
        setMode(m);
    }];
}

export function DiffListSwitch({ mode, onChange }: { mode: DiffListMode; onChange: (m: DiffListMode) => void }) {
    const { t } = useTranslation('projects');
    const button = (value: DiffListMode, label: string, icon: React.ReactNode) => (
        <Tooltip title={label}>
            <ToggleButton value={value} aria-label={label} sx={{ p: 0.5, border: 0, borderRadius: radius.row, color: 'text.disabled', '& svg': { fontSize: 18 }, '&.Mui-selected': { color: 'text.primary', bgcolor: 'action.selected' } }}>
                {icon}
            </ToggleButton>
        </Tooltip>
    );
    return (
        <ToggleButtonGroup exclusive size="small" value={mode} aria-label={t('thread.diff.view.label')} onChange={(_, v: DiffListMode | null) => v && onChange(v)} sx={{ gap: 0.25 }}>
            {button('tree', t('thread.diff.view.tree'), <ViewTree />)}
            {button('list', t('thread.diff.view.list'), <ViewList />)}
        </ToggleButtonGroup>
    );
}
