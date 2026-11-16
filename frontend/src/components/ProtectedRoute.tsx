import React from 'react';
import { Box } from '@mui/material';
import { Navigate, useLocation } from 'react-router-dom';
import { useAuth } from '../contexts/AuthContext';

interface ProtectedRouteProps {
  children: React.ReactNode;
}

const ProtectedRoute: React.FC<ProtectedRouteProps> = ({ children }) => {
  const { isAuthenticated, isLoading } = useAuth();
  const location = useLocation();

  // Show loading with empty screen while checking authentication
  // This prevents any content from flashing before auth check is complete
  if (isLoading) {
    return (
      <Box sx={{
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        height: '100vh',
        bgcolor: 'background.default',
        color: 'text.secondary',
      }}>
        Loading...
      </Box>
    );
  }

  if (!isAuthenticated) {
    // Don't redirect if already on a login page (avoid redirect loops)
    if (location.pathname.startsWith('/login')) {
      return null;
    }
    // Save the original location for redirect after login
    // Use both router state and sessionStorage for persistence across refreshes
    const currentPath = location.pathname + location.search + location.hash;
    sessionStorage.setItem('redirectAfterLogin', currentPath);
    return <Navigate to="/login" state={{ from: location }} replace />;
  }

  return <>{children}</>;
};

export default ProtectedRoute;