// Simple auth failure notification for API layer
// This allows API to notify AuthContext about 401 without managing state

type AuthFailureCallback = () => void;
const listeners: AuthFailureCallback[] = [];

export const authEvents = {
  // Subscribe to auth failure events (401 responses)
  onAuthFailure: (callback: AuthFailureCallback) => {
    listeners.push(callback);
    // Return unsubscribe function
    return () => {
      const index = listeners.indexOf(callback);
      if (index > -1) {
        listeners.splice(index, 1);
      }
    };
  },

  // Notify listeners that auth failed (401 occurred)
  notifyAuthFailure: () => {
    listeners.forEach(fn => fn());
  },
};
