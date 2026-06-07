import React from 'react';
import { ProbeV2Dialog } from './ProbeV2Dialog';
import type { ProbeV2TargetType } from '@/types/probe-v2.ts';

// ProbeV2Menu used to pop a mode-picker menu before opening the dialog. The
// mode/scope selection now lives inside the dialog itself, so this is a thin
// wrapper that opens the dialog directly. `anchorEl` is accepted for caller
// compatibility but unused.
interface ProbeV2MenuProps {
    anchorEl?: HTMLElement | null;
    open: boolean;
    onClose: () => void;
    targetType: ProbeV2TargetType;
    targetId: string;
    targetName: string;
    scenario?: string;
    model?: string;
}

export const ProbeV2Menu: React.FC<ProbeV2MenuProps> = ({
    open,
    onClose,
    targetType,
    targetId,
    targetName,
    scenario,
    model,
}) => (
    <ProbeV2Dialog
        open={open}
        onClose={onClose}
        targetType={targetType}
        targetId={targetId}
        targetName={targetName}
        scenario={scenario}
        model={model}
    />
);

export default ProbeV2Menu;
