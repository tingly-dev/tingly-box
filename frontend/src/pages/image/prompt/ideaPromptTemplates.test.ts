import { describe, expect, it } from 'vitest';
import { DEFAULT_IDEA_INSTRUCTIONS, IDEA_STARTERS, ideaUserMessage } from './ideaPromptTemplates';
import { matchStarter } from './instructionStarters';

describe('ideaUserMessage', () => {
    it('labels the description so it is read as the idea, not an instruction', () => {
        expect(ideaUserMessage('  make it darker \n')).toBe('Description: make it darker');
    });
});

describe('Idea to Prompt starters', () => {
    it('start from the rich one', () => {
        expect(matchStarter(IDEA_STARTERS, DEFAULT_IDEA_INSTRUCTIONS)).toBe('rich');
    });

    it('are told apart, and an edit matches none', () => {
        expect(matchStarter(IDEA_STARTERS, `${IDEA_STARTERS.light}\n`)).toBe('light');
        expect(matchStarter(IDEA_STARTERS, IDEA_STARTERS.tags)).toBe('tags');
        expect(matchStarter(IDEA_STARTERS, `${DEFAULT_IDEA_INSTRUCTIONS}\n- Write in Chinese.`)).toBeNull();
    });

    it('all keep what the person named and answer with the prompt only', () => {
        for (const text of Object.values(IDEA_STARTERS)) {
            expect(text).toMatch(/Output only the/);
        }
        expect(IDEA_STARTERS.rich).toMatch(/Keep every element the user named/);
        expect(IDEA_STARTERS.tags).toMatch(/Keep every element the user named/);
        expect(IDEA_STARTERS.light).toMatch(/Do not translate/);
    });
});
