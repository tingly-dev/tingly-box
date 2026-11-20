import { createContext, useContext } from 'react';

/**
 * True inside the phone shell (`MobileShell`). Pages and shared components
 * switch their phone layout on this, not on the viewport: `?ui=mobile` on a
 * laptop must look like the phone, and a narrow desktop window must not.
 */
export const PhoneShellContext = createContext(false);

export function usePhoneShell(): boolean {
    return useContext(PhoneShellContext);
}
