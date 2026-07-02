import React from 'react';
import { ProbeDialog } from './ProbeDialog';
import type { ProbeTargetType } from '@/types/probe.ts';

// ProbeMenu used to pop a mode-picker menu before opening the dialog. The
// mode/scope selection now lives inside the dialog itself, so this is a thin
// wrapper that opens the dialog directly.
interface ProbeMenuProps {
    open: boolean;
    onClose: () => void;
    targetType: ProbeTargetType;
    targetId: string;
    targetName: string;
    scenario?: string;
    model?: string;
}

export const ProbeMenu: React.FC<ProbeMenuProps> = ({
    open,
    onClose,
    targetType,
    targetId,
    targetName,
    scenario,
    model,
}) => (
    <ProbeDialog
        open={open}
        onClose={onClose}
        targetType={targetType}
        targetId={targetId}
        targetName={targetName}
        scenario={scenario}
        model={model}
    />
);

export default ProbeMenu;
