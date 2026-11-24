import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

vi.mock('react-i18next', () => ({
    useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'en' } }),
}));
vi.mock('../hooks/projectListStore', () => ({
    subscribeProjectList: () => () => undefined,
    getProjectList: () => [],
}));

const toasts: Array<[string, string | undefined]> = [];
vi.mock('../hooks/useNotify', () => ({
    useNotify: () => (message: string, severity?: string) => toasts.push([message, severity]),
}));

class FakeNotification {
    static shown: Array<{ title: string; options?: NotificationOptions }> = [];
    onclick: (() => void) | null = null;
    constructor(title: string, options?: NotificationOptions) {
        FakeNotification.shown.push({ title, options });
    }
    close() {}
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
    FakeNotification.shown = [];
    toasts.length = 0;
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

    it('sends a test notification at the click, asking the browser first when it never was', async () => {
        render(<NotificationControl />);
        fireEvent.click(screen.getByRole('button', { name: 'notifyControl.test' }));
        await waitFor(() => expect(FakeNotification.shown).toHaveLength(1));
        expect(FakeNotification.asked).toBe(1);
        expect(FakeNotification.shown[0].title).toBe('notifyControl.testTitle');
        expect(toasts).toEqual([['notifyControl.test_sent', 'success']]);
        // The grant arrived through the test: the control now shows it on.
        expect(screen.getByRole('switch', { name: 'notifyControl.label' })).toBeChecked();
    });

    it('sends the test without asking again once granted, even with the switch off', async () => {
        FakeNotification.permission = 'granted';
        window.localStorage.setItem('tingly-project.notifications', '0');
        render(<NotificationControl />);
        fireEvent.click(screen.getByRole('button', { name: 'notifyControl.test' }));
        await waitFor(() => expect(FakeNotification.shown).toHaveLength(1));
        expect(FakeNotification.asked).toBe(0);
    });

    it('says so when the test permission is refused or dismissed, and shows nothing', async () => {
        FakeNotification.answer = 'denied';
        render(<NotificationControl />);
        fireEvent.click(screen.getByRole('button', { name: 'notifyControl.test' }));
        await waitFor(() => expect(toasts).toEqual([['notifyControl.test_denied', 'warning']]));
        expect(FakeNotification.shown).toHaveLength(0);
    });

    it('offers no test where it cannot work', () => {
        FakeNotification.permission = 'denied';
        render(<NotificationControl />);
        expect(screen.queryByRole('button', { name: 'notifyControl.test' })).not.toBeInTheDocument();
    });

    it('explains a page that is not a secure context', () => {
        vi.stubGlobal('isSecureContext', false);
        render(<NotificationControl />);
        expect(screen.getByText('notifyControl.insecure')).toBeInTheDocument();
        expect(screen.queryByRole('switch')).not.toBeInTheDocument();
        expect(screen.queryByRole('button', { name: 'notifyControl.test' })).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'notifyControl.howTo' }));
        expect(screen.getByText('notifyControl.insecureBody')).toBeInTheDocument();
    });
});
