import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }),
}));
vi.mock('../hooks/projectListStore', () => ({
    subscribeProjectList: () => () => undefined,
    getProjectList: () => [],
}));

class FakeNotification {
    static permission: NotificationPermission = 'default';
    static asked = 0;
    static answer: NotificationPermission = 'granted';
    static requestPermission = async () => {
        FakeNotification.asked++;
        FakeNotification.permission = FakeNotification.answer;
        return FakeNotification.answer;
    };
}

import { NotificationControl } from './NotificationControl';
import { notificationsPreferred } from './engine';

beforeEach(() => {
    vi.stubGlobal('Notification', FakeNotification);
    FakeNotification.permission = 'default';
    FakeNotification.asked = 0;
    FakeNotification.answer = 'granted';
    window.localStorage.removeItem('tingly-project.notifications');
});
afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
});

describe('NotificationControl', () => {
    it('offers Enable first and asks the browser only at the click', async () => {
        render(<NotificationControl />);
        // Default on, but nothing was asked on load.
        expect(FakeNotification.asked).toBe(0);
        fireEvent.click(screen.getByRole('button', { name: 'notifyControl.enable' }));
        await waitFor(() => expect(screen.getByRole('switch', { name: 'notifyControl.label' })).toBeChecked());
        expect(FakeNotification.asked).toBe(1);
    });

    it('turns off and on with the switch once granted, without asking again', async () => {
        FakeNotification.permission = 'granted';
        render(<NotificationControl />);
        const toggle = screen.getByRole('switch', { name: 'notifyControl.label' });
        expect(toggle).toBeChecked();
        fireEvent.click(toggle);
        expect(toggle).not.toBeChecked();
        expect(notificationsPreferred()).toBe(false);
        fireEvent.click(toggle);
        await waitFor(() => expect(toggle).toBeChecked());
        expect(FakeNotification.asked).toBe(0);
    });

    it('says when the browser blocks it and explains how to unblock', () => {
        FakeNotification.permission = 'denied';
        render(<NotificationControl />);
        expect(screen.getByText('notifyControl.blocked')).toBeInTheDocument();
        expect(screen.queryByRole('switch')).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'notifyControl.howTo' }));
        expect(screen.getByText('notifyControl.blockedBody')).toBeInTheDocument();
    });

    it('keeps the choice "on" when the dialog is dismissed, and offers Enable again', async () => {
        FakeNotification.answer = 'default';
        render(<NotificationControl />);
        fireEvent.click(screen.getByRole('button', { name: 'notifyControl.enable' }));
        await waitFor(() => expect(FakeNotification.asked).toBe(1));
        expect(await screen.findByRole('button', { name: 'notifyControl.enable' })).toBeEnabled();
    });

    it('is a plain word where the browser cannot notify', () => {
        vi.stubGlobal('Notification', undefined);
        delete (window as { Notification?: unknown }).Notification;
        render(<NotificationControl />);
        expect(screen.getByText('notifyControl.unsupported')).toBeInTheDocument();
        expect(screen.queryByRole('button')).not.toBeInTheDocument();
    });
});
