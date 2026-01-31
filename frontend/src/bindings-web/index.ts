// Mock bindings for non-GUI builds
// This file provides mock implementations when Wails bindings are not available


export const TinglyService = {
    GetGinEngine: async () => {
        console.warn('[Mock] TinglyService.GetGinEngine called - returning null');
        return null;
    },
    GetPort: async () => {
        console.warn('[Mock] TinglyService.GetPort called - returning 8080');
        return 8080;
    },
    GetUserAuthToken: async () => {
        console.warn('[Mock] TinglyService.GetUserAuthToken called - returning empty string');
        return '';
    },
    Start: async () => {
        console.warn('[Mock] TinglyService.Start called - no-op');
    },
    Stop: async () => {
        console.warn('[Mock] TinglyService.Stop called - no-op');
    }
};

export const GreetService = {
    Greet: async (name: string) => {
        console.warn(`[Mock] GreetService.Greet called with name: ${name}`);
        return `Hello, ${name}! (Mock)`;
    }
};

// Mock Events for web mode (systray not available in web)
export const Events = {
    On: (eventName: string, callback: (event: any) => void) => {
        console.warn(`[Mock] Events.On called for event: ${eventName} - no-op in web mode`);
        return () => {}; // Return no-op cleanup function
    },
    Off: (eventName: string) => {
        console.warn(`[Mock] Events.Off called for event: ${eventName} - no-op in web mode`);
    },
    Emit: (eventName: string, data?: any) => {
        console.warn(`[Mock] Events.Emit called for event: ${eventName} - no-op in web mode`);
        return Promise.resolve(false);
    }
};

export default TinglyService;