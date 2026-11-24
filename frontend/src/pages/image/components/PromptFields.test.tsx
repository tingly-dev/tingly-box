import { render, screen } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { PhoneShellContext } from '@tingly/ui/hooks/usePhoneShell';
import { PromptPanelField } from './PromptFields';
import type { PromptStore } from './usePromptStore';

const store = (text: string): PromptStore => ({ get: () => text, set: () => undefined, subscribe: () => () => undefined });

const field = (
    <PromptPanelField
        store={store('a lighthouse at dusk')}
        inputRef={null}
        placeholder="Describe the image"
        ariaLabel="Prompt"
        disabled={false}
        onKeyDown={() => undefined}
        onDrop={() => undefined}
        actions={<button>expand</button>}
        copyLabel="Copy"
        copiedLabel="Copied"
    />
);

describe('PromptPanelField', () => {
    it('puts its buttons on a row above the text in the phone shell', () => {
        render(<PhoneShellContext.Provider value>{field}</PhoneShellContext.Provider>);
        const root = screen.getByRole('textbox', { name: 'Prompt' }).closest('.MuiInputBase-root') as HTMLElement;
        expect(root).toHaveStyle({ display: 'grid' });
        expect(screen.getByRole('button', { name: 'expand' }).closest('.MuiInputAdornment-root')).toHaveStyle({ order: '-1' });
    });

    it('keeps them in a column beside the text on the desktop', () => {
        render(field);
        const root = screen.getByRole('textbox', { name: 'Prompt' }).closest('.MuiInputBase-root') as HTMLElement;
        expect(root).not.toHaveStyle({ display: 'grid' });
    });
});
