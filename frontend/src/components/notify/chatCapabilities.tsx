import {Bell as NotifyIcon, CheckCircle as ConfirmIcon, ListAlt as ChooseIcon, Message as AskIcon} from '@/components/icons';
import type {ChatCapability} from './useChatProbe';

// chatCapabilities is the single source of truth for the per-chat capability
// probe row — what buttons render, in what order, their labels, and which are
// gated. The probe feature is mode-driven via a ToggleButtonGroup
// (ProbeDialog.tsx); chat has more capabilities, so this is the preset array
// the probe lacks. See the IM Notify probe plan.
//
// `gated` capabilities are shown disabled with an experimental tooltip: their
// backend option/free-text threading into the IM keyboard is incomplete
// (imchannel.ToAskRequest doesn't copy ix.Options), so we don't expose them
// until that's fixed — no point wiring a trigger that can't render its prompt.

export interface ChatCapabilityDef {
    capability: ChatCapability | 'choose' | 'ask';
    label: string;
    icon: React.ReactNode;
    /** Disabled with an "experimental" tooltip — backend path incomplete. */
    gated?: boolean;
    /** One-line hint for the button tooltip. */
    hint: string;
}

export const CHAT_CAPABILITIES: ChatCapabilityDef[] = [
    {
        capability: 'notify',
        label: 'Notify',
        icon: <NotifyIcon fontSize="small" />,
        hint: 'Send a one-way test notification and verify delivery',
    },
    {
        capability: 'confirm',
        label: 'Confirm',
        icon: <ConfirmIcon fontSize="small" />,
        hint: 'Send an Allow/Deny prompt and wait for the reply',
    },
    {
        capability: 'choose',
        label: 'Choose',
        icon: <ChooseIcon fontSize="small" />,
        gated: true,
        hint: 'Multi-option prompt (experimental — backend option rendering incomplete)',
    },
    {
        capability: 'ask',
        label: 'Ask',
        icon: <AskIcon fontSize="small" />,
        gated: true,
        hint: 'Free-text question (experimental — backend text capture incomplete)',
    },
];
