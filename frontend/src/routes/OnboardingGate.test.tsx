import { cleanup, render, screen } from '@testing-library/react';
import { MemoryRouter, Route, Routes } from 'react-router-dom';
import { afterEach, describe, expect, it } from 'vitest';
import OnboardingGate from './OnboardingGate';

afterEach(cleanup);

describe('first-run landing', () => {
    it('always lands on the agent page, with or without providers', () => {
        render(
            <MemoryRouter initialEntries={['/']}>
                <Routes>
                    <Route path="/" element={<OnboardingGate />} />
                    <Route path="/help" element={<div>Help</div>} />
                    <Route path="/agent" element={<div>Agent setup</div>} />
                </Routes>
            </MemoryRouter>,
        );
        expect(screen.getByText('Agent setup')).toBeVisible();
    });
});
