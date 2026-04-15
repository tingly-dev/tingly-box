// Type declarations for Wails bindings (only available in GUI builds)
// This file provides type stubs for TypeScript checking in web builds

declare module '../bindings/github.com/tingly-dev/tingly-box/gui/wails3/services' {
    interface TinglyService {
        GetGinEngine(): Promise<any>;
        GetPort(): Promise<number>;
        GetUserAuthToken(): Promise<string>;
        Start(): Promise<void>;
        Stop(): Promise<void>;
    }

    const TinglyService: TinglyService;
    export default TinglyService;
}

declare module '@wailsio/runtime' {
    interface Events {
        On(eventName: string, callback: (event: any) => void): () => void;
        Off(eventName: string): void;
        Emit(eventName: string, data?: any): Promise<boolean>;
    }

    export const Events: Events;
}
