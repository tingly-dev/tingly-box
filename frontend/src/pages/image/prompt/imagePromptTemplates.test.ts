import { describe, expect, it } from 'vitest';
import { DEFAULT_INSTRUCTIONS, INSTRUCTION_STARTERS, cleanPromptReply, matchingStarter } from './imagePromptTemplates';

describe('cleanPromptReply', () => {
    it('keeps a plain reply as it is', () => {
        expect(cleanPromptReply('  A red fox asleep in snow, watercolor.  ')).toBe('A red fox asleep in snow, watercolor.');
    });

    it('takes off a code fence', () => {
        expect(cleanPromptReply('```text\nA red fox, watercolor\n```')).toBe('A red fox, watercolor');
        expect(cleanPromptReply('```\nfox, snow\n```')).toBe('fox, snow');
    });

    it('takes off a "Prompt:" label', () => {
        expect(cleanPromptReply('Prompt: A red fox')).toBe('A red fox');
        expect(cleanPromptReply('**Prompt**: A red fox')).toBe('A red fox');
    });

    it('takes off quotes around the whole reply, not quotes inside it', () => {
        expect(cleanPromptReply('"A red fox in snow"')).toBe('A red fox in snow');
        expect(cleanPromptReply('A sign that reads "OPEN", neon')).toBe('A sign that reads "OPEN", neon');
        expect(cleanPromptReply('"A sign reading "OPEN" at night"')).toBe('"A sign reading "OPEN" at night"');
    });
});

describe('matchingStarter', () => {
    it('recognises each starter, whitespace aside', () => {
        expect(matchingStarter(DEFAULT_INSTRUCTIONS)).toBe('natural');
        expect(matchingStarter(`\n${INSTRUCTION_STARTERS.tags}  `)).toBe('tags');
        expect(matchingStarter(INSTRUCTION_STARTERS.style)).toBe('style');
    });

    it('returns null once the text is edited', () => {
        expect(matchingStarter(`${DEFAULT_INSTRUCTIONS}\n- Write in Chinese.`)).toBeNull();
        expect(matchingStarter('')).toBeNull();
    });
});
