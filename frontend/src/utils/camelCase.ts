/**
 * Convert a string from snake_case to camelCase
 */
export const toCamelCase = (str: string): string => {
    return str.replace(/_([a-z])/g, (match, letter) => letter.toUpperCase());
};

/**
 * Recursively convert all object keys from snake_case to camelCase
 */
export const keysToCamelCase = <T = any>(obj: any): T => {
    if (obj === null || obj === undefined) {
        return obj;
    }

    // Handle arrays
    if (Array.isArray(obj)) {
        return obj.map(item => keysToCamelCase(item)) as T;
    }

    // Handle objects
    if (typeof obj === 'object') {
        const result: any = {};
        for (const key in obj) {
            if (obj.hasOwnProperty(key)) {
                const camelKey = toCamelCase(key);
                result[camelKey] = keysToCamelCase(obj[key]);
            }
        }
        return result as T;
    }

    // Return primitive values as-is
    return obj;
};

/**
 * Convert camelCase to snake_case (if needed for sending data back to server)
 */
export const toSnakeCase = (str: string): string => {
    return str.replace(/[A-Z]/g, letter => `_${letter.toLowerCase()}`);
};

/**
 * Recursively convert all object keys from camelCase to snake_case
 */
export const keysToSnakeCase = <T = any>(obj: any): T => {
    if (obj === null || obj === undefined) {
        return obj;
    }

    // Handle arrays
    if (Array.isArray(obj)) {
        return obj.map(item => keysToSnakeCase(item)) as T;
    }

    // Handle objects
    if (typeof obj === 'object') {
        const result: any = {};
        for (const key in obj) {
            if (obj.hasOwnProperty(key)) {
                const snakeKey = toSnakeCase(key);
                result[snakeKey] = keysToSnakeCase(obj[key]);
            }
        }
        return result as T;
    }

    // Return primitive values as-is
    return obj;
};