// Claude Code model slots: the env vars through which Claude Code picks the
// model id it sends. Each slot is bound to one rule; the rule's request_model
// is what lands in the env. See .design/claude-code-slot-binding.md.

export const CLAUDE_CODE_SLOTS = ['default', 'haiku', 'sonnet', 'opus', 'fable', 'subagent'] as const;
export type ClaudeCodeSlot = typeof CLAUDE_CODE_SLOTS[number];

export const CLAUDE_CODE_SLOT_ENV: Record<ClaudeCodeSlot, string> = {
    default: 'ANTHROPIC_MODEL',
    haiku: 'ANTHROPIC_DEFAULT_HAIKU_MODEL',
    sonnet: 'ANTHROPIC_DEFAULT_SONNET_MODEL',
    opus: 'ANTHROPIC_DEFAULT_OPUS_MODEL',
    fable: 'ANTHROPIC_DEFAULT_FABLE_MODEL',
    subagent: 'CLAUDE_CODE_SUBAGENT_MODEL',
};

/** The main Claude Code rule: where every slot goes unless bound elsewhere. */
export const CLAUDE_CODE_MAIN_RULE_UUID = 'builtin:claude_code:cc';

/** One slot as the backend resolves it (GET …/claude-code/slots). */
export interface ClaudeCodeSlotResolution {
    slot: ClaudeCodeSlot;
    /** Effective rule; empty when no active rule resolves. */
    rule_uuid: string;
    request_model: string;
    context_1m: boolean;
    /** The slot has its own binding (false: it follows the default slot). */
    bound: boolean;
}

/** Slot → UUID of the rule it requests. */
export type ClaudeCodeSlotRules = Partial<Record<ClaudeCodeSlot, string>>;

export const slotRulesOf = (slots: ClaudeCodeSlotResolution[]): ClaudeCodeSlotRules =>
    Object.fromEntries(slots.filter(s => s.rule_uuid).map(s => [s.slot, s.rule_uuid]));

/** Whether every slot requests the same rule. */
export const slotsUnified = (slots: ClaudeCodeSlotResolution[]): boolean =>
    slots.every(s => s.rule_uuid === slots[0]?.rule_uuid && s.request_model === slots[0]?.request_model);
