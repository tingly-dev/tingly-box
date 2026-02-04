import React, { useEffect, useState } from 'react';
import {
  Box,
  Card,
  CardContent,
  Typography,
  Button,
  TextField,
  Alert,
  Chip,
  List,
  ListItem,
  ListItemText,
  ListItemIcon,
  Divider,
  Skeleton,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
} from '@mui/material';
import {
  Security as SecurityIcon,
  Block as BlockIcon,
  Warning as WarningIcon,
  Refresh as RefreshIcon,
  Info as InfoIcon,
} from '@mui/icons-material';
import { useTheme, alpha } from '@mui/material/styles';
import { getRateLimitStats, resetRateLimit } from '@/services/api';

interface RateLimitStatsData {
  stats: {
    total_ips_tracked: number;
    currently_blocked: number;
    max_attempts: number;
    window_size: string;
    block_duration: string;
  };
}

export const RateLimitPage: React.FC = () => {
  const theme = useTheme();
  const [stats, setStats] = useState<RateLimitStatsData['stats'] | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [resetDialogOpen, setResetDialogOpen] = useState(false);
  const [resetIp, setResetIp] = useState('');
  const [resetLoading, setResetLoading] = useState(false);
  const [resetMessage, setResetMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  const fetchStats = async () => {
    try {
      const data = await getRateLimitStats();
      setStats(data.stats);
    } catch (err) {
      setError('Failed to load rate limit statistics');
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchStats();
  }, []);

  const handleResetRateLimit = async () => {
    if (!resetIp.trim()) return;

    setResetLoading(true);
    setResetMessage(null);

    try {
      const result = await resetRateLimit(resetIp.trim());
      setResetMessage({ type: 'success', text: result.message });
      setResetIp('');
      fetchStats();
    } catch (err) {
      setResetMessage({ type: 'error', text: 'Failed to reset rate limit' });
    } finally {
      setResetLoading(false);
    }
  };

  if (loading) {
    return (
      <Box>
        <Typography variant="h4" fontWeight={700} gutterBottom>
          Rate Limit Management
        </Typography>
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 3, mt: 2 }}>
          {[1, 2, 3, 4].map((i) => (
            <Box key={i} sx={{ width: { xs: '100%', sm: 'calc(50% - 12px)', md: 'calc(25% - 18px)' } }}>
              <Skeleton variant="rounded" height={160} />
            </Box>
          ))}
        </Box>
      </Box>
    );
  }

  if (error) {
    return (
      <Box sx={{ textAlign: 'center', py: 8 }}>
        <Typography variant="h6" color="error">
          {error}
        </Typography>
      </Box>
    );
  }

  return (
    <Box>
      <Box sx={{ mb: 3 }}>
        <Typography variant="h4" fontWeight={700} gutterBottom>
          Rate Limit Management
        </Typography>
        <Typography variant="body1" color="text.secondary">
          Monitor and manage API rate limiting settings
        </Typography>
      </Box>

      {/* Stats Cards */}
      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 3, mb: 4 }}>
        <Box sx={{ width: { xs: '100%', sm: 'calc(50% - 12px)', md: 'calc(25% - 18px)' } }}>
          <Card sx={{ height: '100%' }}>
            <CardContent sx={{ p: 2 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, mb: 2 }}>
                <SecurityIcon color="primary" />
                <Typography variant="subtitle1" fontWeight={600}>
                  Configuration
                </Typography>
              </Box>
              <Typography variant="h4" fontWeight={700}>
                {stats?.max_attempts || 5}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Max attempts per window
              </Typography>
            </CardContent>
          </Card>
        </Box>

        <Box sx={{ width: { xs: '100%', sm: 'calc(50% - 12px)', md: 'calc(25% - 18px)' } }}>
          <Card sx={{ height: '100%' }}>
            <CardContent sx={{ p: 2 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, mb: 2 }}>
                <InfoIcon color="info" />
                <Typography variant="subtitle1" fontWeight={600}>
                  Time Window
                </Typography>
              </Box>
              <Typography variant="h4" fontWeight={700}>
                {stats?.window_size?.replace('m', 'm ') || '5m'}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Duration before reset
              </Typography>
            </CardContent>
          </Card>
        </Box>

        <Box sx={{ width: { xs: '100%', sm: 'calc(50% - 12px)', md: 'calc(25% - 18px)' } }}>
          <Card sx={{ height: '100%' }}>
            <CardContent sx={{ p: 2 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, mb: 2 }}>
                <BlockIcon color="warning" />
                <Typography variant="subtitle1" fontWeight={600}>
                  Block Duration
                </Typography>
              </Box>
              <Typography variant="h4" fontWeight={700}>
                {stats?.block_duration?.replace('m', 'm ') || '5m'}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Time blocked after limit
              </Typography>
            </CardContent>
          </Card>
        </Box>

        <Box sx={{ width: { xs: '100%', sm: 'calc(50% - 12px)', md: 'calc(25% - 18px)' } }}>
          <Card
            sx={{
              height: '100%',
              bgcolor: (stats?.currently_blocked || 0) > 0
                ? alpha(theme.palette.warning.main, 0.1)
                : undefined,
            }}
          >
            <CardContent sx={{ p: 2 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5, mb: 2 }}>
                <WarningIcon
                  color={(stats?.currently_blocked || 0) > 0 ? 'warning' : 'success'}
                />
                <Typography variant="subtitle1" fontWeight={600}>
                  Currently Blocked
                </Typography>
              </Box>
              <Typography variant="h4" fontWeight={700}>
                {stats?.currently_blocked || 0}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                IPs blocked now
              </Typography>
            </CardContent>
          </Card>
        </Box>
      </Box>

      {/* Reset Rate Limit */}
      <Card sx={{ mb: 3 }}>
        <CardContent sx={{ p: 2 }}>
          <Typography variant="h6" fontWeight={600} gutterBottom>
            Reset Rate Limit for IP
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Manually reset rate limiting for a specific IP address that has been blocked.
          </Typography>

          {resetMessage && (
            <Alert
              severity={resetMessage.type}
              sx={{ mb: 2 }}
              onClose={() => setResetMessage(null)}
            >
              {resetMessage.text}
            </Alert>
          )}

          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
            <TextField
              label="IP Address"
              placeholder="e.g., 192.168.1.100"
              value={resetIp}
              onChange={(e) => setResetIp(e.target.value)}
              size="small"
              fullWidth
            />
            <Button
              variant="contained"
              startIcon={<RefreshIcon />}
              onClick={() => setResetDialogOpen(true)}
              disabled={!resetIp.trim()}
              fullWidth
            >
              Reset
            </Button>
          </Box>
        </CardContent>
      </Card>

      {/* How Rate Limiting Works */}
      <Card>
        <CardContent sx={{ p: 2 }}>
          <Typography variant="h6" fontWeight={600} gutterBottom>
            How Rate Limiting Works
          </Typography>
          <List dense>
            <ListItem>
              <ListItemIcon>
                <Chip label="1" size="small" color="primary" />
              </ListItemIcon>
              <ListItemText
                primary="Authentication Attempts"
                secondary="Users have limited attempts within the time window."
              />
            </ListItem>
            <Divider />
            <ListItem>
              <ListItemIcon>
                <Chip label="2" size="small" color="primary" />
              </ListItemIcon>
              <ListItemText
                primary="IP Blocking"
                secondary="After exceeding max attempts, IP is blocked."
              />
            </ListItem>
            <Divider />
            <ListItem>
              <ListItemIcon>
                <Chip label="3" size="small" color="primary" />
              </ListItemIcon>
              <ListItemText
                primary="Automatic Unblock"
                secondary="Blocked IPs are auto-unblocked after duration."
              />
            </ListItem>
            <Divider />
            <ListItem>
              <ListItemIcon>
                <Chip label="4" size="small" color="primary" />
              </ListItemIcon>
              <ListItemText
                primary="Manual Reset"
                secondary="Admins can manually reset rate limits."
              />
            </ListItem>
          </List>
        </CardContent>
      </Card>

      {/* Reset Confirmation Dialog */}
      <Dialog open={resetDialogOpen} onClose={() => setResetDialogOpen(false)}>
        <DialogTitle>Confirm Rate Limit Reset</DialogTitle>
        <DialogContent>
          <Typography>
            Are you sure you want to reset the rate limit for IP: <strong>{resetIp}</strong>?
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setResetDialogOpen(false)}>Cancel</Button>
          <Button
            variant="contained"
            onClick={() => {
              handleResetRateLimit();
              setResetDialogOpen(false);
            }}
            disabled={resetLoading}
          >
            Confirm Reset
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};
