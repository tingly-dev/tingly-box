import { useState } from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { beforeAll, beforeEach, describe, expect, it } from 'vitest';
import i18n from '@/i18n';
import MCPPasteImport from './MCPPasteImport';
import { defaultMCPSourceFormValue, type MCPSourceFormValue } from '../../features/mcp/types';

beforeAll(async () => {
    await i18n.changeLanguage('en');
});

let latest: MCPSourceFormValue;
function Harness() {
    const [form, setForm] = useState(defaultMCPSourceFormValue);
    return (
        <MCPPasteImport
            value={form}
            onChange={(next) => {
                latest = next;
                setForm(next);
            }}
        />
    );
}

const box = () => screen.getByRole('textbox', { name: 'Paste a command or config' });

describe('MCPPasteImport', () => {
    beforeEach(() => {
        latest = undefined as unknown as MCPSourceFormValue;
    });

    it('fills the form when a command is pasted', () => {
        render(<Harness />);
        fireEvent.paste(box());
        fireEvent.change(box(), { target: { value: 'npx -y @modelcontextprotocol/server-filesystem "/My Docs"' } });
        expect(latest).toMatchObject({
            name: 'server-filesystem',
            transport: 'stdio',
            command: 'npx',
            args: ['-y', '@modelcontextprotocol/server-filesystem', '/My Docs'],
        });
        expect(screen.getByText(/Read as command line: npx · 3 arguments/)).toBeInTheDocument();
    });

    it('lets the user pick one of several servers', () => {
        render(<Harness />);
        fireEvent.change(box(), {
            target: {
                value: '{"mcpServers": {"a": {"command": "uvx", "args": ["a"]}, "b": {"url": "https://b.test/mcp"}}}',
            },
        });
        fireEvent.click(screen.getByRole('button', { name: 'Fill in fields' }));
        expect(latest).toMatchObject({ name: 'a', command: 'uvx' });
        fireEvent.click(screen.getByRole('button', { name: 'b' }));
        expect(latest).toMatchObject({ name: 'b', transport: 'http', endpoint: 'https://b.test/mcp' });
    });

    it('keeps the text and the form when the paste cannot be read', () => {
        render(<Harness />);
        fireEvent.paste(box());
        fireEvent.change(box(), { target: { value: '{"mcpServers": {' } });
        expect(screen.getByText(/This looks like JSON but cannot be read/)).toBeInTheDocument();
        expect(box()).toHaveValue('{"mcpServers": {');
        expect(latest).toBeUndefined();
    });
});
