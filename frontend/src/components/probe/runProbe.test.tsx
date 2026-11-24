import { render, screen } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import { controlApi } from '@/services/openapi';
import { runProbe } from './runProbe';
import { StatusBar } from './ResultSections';

vi.mock('@/services/openapi', () => ({ controlApi: vi.fn() }));

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }));

it('renders the complete original error body from a failed probe', async () => {
    const responseBody = '{\n  "error": "' + 'diagnostic '.repeat(500) + '"\n}\n';
    vi.mocked(controlApi).mockResolvedValue({
        success: false,
        error: { message: '400 Bad Request', type: 'probe_error', response_body: responseBody },
    });
    const result = await runProbe({ target_type: 'provider' });
    render(<StatusBar result={result} />);
    expect(screen.getByRole('alert').textContent).toContain(responseBody);
});
