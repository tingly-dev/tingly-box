// Type declarations for Wails runtime (only available in GUI builds)
// This file provides type stubs for TypeScript checking in web builds

declare module '@wailsio/runtime' {
    interface Events {
        On(eventName: string, callback: (event: any) => void): () => void;
        Off(eventName: string): void;
        Emit(eventName: string, data?: any): Promise<boolean>;
    }

    export const Events: Events;
}
