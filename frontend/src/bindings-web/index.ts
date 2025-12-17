// Mock bindings for non-GUI builds
// This file provides mock implementations when Wails bindings are not available


export const ProxyService = {
    GetGinEngine: async () => {
        console.warn('[Mock] ProxyService.GetGinEngine called - returning null');
        return null;
    },
    GetPort: async () => {
        console.warn('[Mock] ProxyService.GetPort called - returning 8080');
        return 8080;
    },
    GetUserAuthToken: async () => {
        console.warn('[Mock] ProxyService.GetUserAuthToken called - returning empty string');
        return '';
    },
    Start: async () => {
        console.warn('[Mock] ProxyService.Start called - no-op');
    },
    Stop: async () => {
        console.warn('[Mock] ProxyService.Stop called - no-op');
    }
};

export const GreetService = {
    Greet: async (name: string) => {
        console.warn(`[Mock] GreetService.Greet called with name: ${name}`);
        return `Hello, ${name}! (Mock)`;
    }
};

export default ProxyService;