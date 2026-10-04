import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '@/i18n';
import { AgentInstallCard } from './AgentInstallCard';

vi.mock('@/utils/protocol', () => ({ getApiBaseUrl: () => Promise.resolve('http://localhost:12580') }));

beforeEach(async () => {
    await i18n.changeLanguage('zh');
});

describe('MCP client installation', () => {
    it('uses the selected profile endpoint and the supported Codex token option', async () => {
        render(<AgentInstallCard clientId="codex-dev" defaultRuntime="codex" />);
        await waitFor(() =>
            expect(screen.getByText(/codex mcp add tb --url/)).toHaveTextContent(
                'codex mcp add tb --url "http://localhost:12580/api/v1/mcp/codex-dev" --bearer-token-env-var TINGLY_MCP_TOKEN'
            )
        );
        expect(screen.getByRole('button', { name: 'Codex' })).toHaveAttribute('aria-pressed', 'true');
        expect(screen.getByRole('button', { name: 'Claude Code' })).toHaveAttribute('aria-pressed', 'false');
        expect(screen.getByText('添加到客户端')).toBeInTheDocument();
        expect(screen.getByText('运行 Codex 前，请将 TINGLY_MCP_TOKEN 环境变量设为你的用户令牌。')).toBeInTheDocument();
    });

    it('gives OpenCode its own token instructions and preserves custom headings', async () => {
        render(<AgentInstallCard heading="接入说明" clientId="restricted" />);
        fireEvent.click(screen.getByRole('button', { name: 'OpenCode' }));
        expect(screen.getByText('接入说明')).toBeInTheDocument();
        expect(screen.getByText('OpenCode 从 MY_API_KEY 环境变量读取令牌。')).toBeInTheDocument();
        await waitFor(() =>
            expect(screen.getByText(/"type": "remote"/)).toHaveTextContent(
                '"url": "http://localhost:12580/api/v1/mcp/restricted"'
            )
        );
        expect(screen.getByText(/"type": "remote"/)).toHaveTextContent('Bearer {env:MY_API_KEY}');
        expect(screen.getByText(/请将 MY_API_KEY 环境变量设为你的用户令牌/)).toBeInTheDocument();
    });
});
