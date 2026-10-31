import { render, screen, within } from '@testing-library/react';
import { createTheme, ThemeProvider } from '@mui/material';
import { describe, expect, it } from 'vitest';
import catalog from '../../../internal/catalog/providers.json';
import { providerIconId } from '@/utils/providerIcon';
import ProviderIcon from './ProviderIcon';
import { FALLBACK_OAUTH_PROVIDERS } from './oauth/fallbackProviders';

describe('provider icon coverage', () => {
    it('renders a brand image for every catalog entry', () => {
        render(<>{Object.values(catalog.providers).map(provider => (
            <div key={provider.id} data-testid={provider.id}>
                <ProviderIcon identifier={provider.icon || provider.id} />
            </div>
        ))}</>);
        for (const provider of Object.values(catalog.providers)) {
            expect(within(screen.getByTestId(provider.id)).getByRole('img')).toHaveAttribute('alt');
        }
    });

    it('resolves every real OAuth issuer and the ChatGPT alias to a brand image', () => {
        const issuers = FALLBACK_OAUTH_PROVIDERS.filter(p => !p.dev).map(p => p.id);
        render(<>{[...issuers, 'chatgpt'].map(issuer => (
            <div key={issuer} data-testid={issuer}>
                <ProviderIcon identifier={providerIconId({ oauth_detail: { issuer } })!} />
            </div>
        ))}</>);
        for (const issuer of [...issuers, 'chatgpt']) {
            expect(within(screen.getByTestId(issuer)).getByRole('img')).toHaveAttribute('alt');
        }
        expect(within(screen.getByTestId('chatgpt')).getByRole('img')).toHaveAttribute('alt', 'Codex');
    });

    it.each(['light', 'dark'] as const)('keeps monochrome local brands legible in %s mode', mode => {
        render(<ThemeProvider theme={createTheme({ palette: { mode } })}>
            {['ollama', 'lm-studio', 'amd', 'vercel'].map(identifier => (
                <ProviderIcon key={identifier} identifier={identifier} />
            ))}
        </ThemeProvider>);
        for (const image of screen.getAllByRole('img')) {
            expect(image).toHaveStyle({ filter: mode === 'dark' ? 'invert(1)' : 'none' });
        }
    });

    it('retains the neutral placeholder for unknown providers', () => {
        render(<ProviderIcon identifier="unknown-provider" />);
        expect(screen.queryByRole('img')).not.toBeInTheDocument();
    });
});
