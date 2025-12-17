// Type definitions for ProxyService interface
export interface ProxyServiceType {
    GetGinEngine(): Promise<any | null>;
    GetPort(): Promise<number>;
    GetUserAuthToken(): Promise<string>;
    Start(): Promise<void>;
    Stop(): Promise<void>;
}

// Cache for dynamically imported ProxyService
let ProxyService: ProxyServiceType | null = null;
let importPromise: Promise<ProxyServiceType | null> | null = null;

// Helper function to dynamically import ProxyService when needed
export const getProxyService = async (): Promise<ProxyServiceType | null> => {
    // Return cached service if available
    if (ProxyService) {
        return ProxyService;
    }

    // If import is already in progress, return the existing promise
    if (importPromise) {
        return importPromise;
    }

    // Check if we're in GUI mode
    const isGuiMode = import.meta.env.VITE_PKG_MODE === "gui";

    if (!isGuiMode) {
        return null;
    }

    // Start the import process
    importPromise = (async (): Promise<ProxyServiceType | null> => {
        try {
            // Use dynamic import with Vite's @vite-ignore to prevent static analysis
            const modulePath = '../bindings/tingly-box/internal/wails3/services';
            const module = await import(/* @vite-ignore */ modulePath);
            ProxyService = module.ProxyService as ProxyServiceType;
            importPromise = null; // Clear the promise after successful import
            return ProxyService;
        } catch (err) {
            console.error('Failed to load ProxyService:', err);
            ProxyService = null;
            importPromise = null; // Clear the promise after failed import
            return null;
        }
    })();

    return importPromise;
};
