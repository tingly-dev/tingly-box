import React, { useEffect, useState } from 'react';
import {
  Box,
  Card,
  CardContent,
  Typography,
  List,
  ListItem,
  ListItemText,
  ListItemIcon,
  Skeleton,
  Divider,
} from '@mui/material';
import {
  Analytics as SessionIcon,
  CheckCircle as CheckIcon,
  Error as ErrorIcon,
  Cancel as CancelIcon,
  PlayArrow as RunningIcon,
  AccessTime as TimeIcon,
  Security as SecurityIcon,
} from '@mui/icons-material';
import { alpha } from '@mui/material/styles';
import { getStats } from '@/services/api';
import type { StatsResponse } from '@/types';

const formatDuration = (str: string): string => {
  const match = str.match(/(?:(\d+)h)?(\d+)m(\d+)s/);
  if (match) {
    const [, hours, minutes, seconds] = match;
    const parts: string[] = [];
    if (hours) parts.push(`${hours}h`);
    if (minutes) parts.push(`${minutes}m`);
    parts.push(`${seconds}s`);
    return parts.join(' ');
  }
  return str;
};

interface StatCardProps {
  title: string;
  value: string | number;
  icon: React.ReactNode;
  color: string;
}

const StatCard: React.FC<StatCardProps> = ({ title, value, icon, color }) => {
  return (
    <Card sx={{ height: '100%' }}>
      <CardContent sx={{ p: 2 }}>
        <Box sx={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 1 }}>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Typography variant="body2" color="text.secondary" gutterBottom noWrap>
              {title}
            </Typography>
            <Typography
              variant="h5"
              sx={{ fontSize: { xs: '1.5rem', sm: '2rem' }, fontWeight: 700, lineHeight: 1.2 }}
            >
              {value}
            </Typography>
          </Box>
          <Box
            sx={{
              p: 1,
              borderRadius: 1.5,
              bgcolor: alpha(color, 0.1),
              color: color,
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              flexShrink: 0,
            }}
          >
            {icon}
          </Box>
        </Box>
      </CardContent>
    </Card>
  );
};

export const DashboardPage: React.FC = () => {
  const [stats, setStats] = useState<StatsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const fetchStats = async () => {
      try {
        const data = await getStats();
        setStats(data);
      } catch (err) {
        setError('Failed to load statistics');
        console.error(err);
      } finally {
        setLoading(false);
      }
    };
    fetchStats();
    const interval = setInterval(fetchStats, 30000);
    return () => clearInterval(interval);
  }, []);

  if (loading) {
    return (
      <Box>
        <Typography variant="h4" fontWeight={700} gutterBottom>
          Dashboard
        </Typography>
        <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 2, mt: 2 }}>
          {[1, 2, 3, 4].map((i) => (
            <Box key={i} sx={{ width: { xs: 'calc(50% - 8px)', md: 'calc(25% - 18px)' } }}>
              <Skeleton variant="rounded" height={120} />
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
      <Typography variant="h4" fontWeight={700} gutterBottom sx={{ fontSize: { xs: '1.75rem', sm: '2.125rem' } }}>
        Dashboard
      </Typography>
      <Typography variant="body1" color="text.secondary" sx={{ mb: 3 }}>
        Overview of OPSX service statistics and performance
      </Typography>

      {/* Stat Cards - 2x2 on mobile, 4x1 on desktop */}
      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 2, mb: 3 }}>
        <Box sx={{ width: { xs: 'calc(50% - 8px)', md: 'calc(25% - 18px)' } }}>
          <StatCard
            title="Total Sessions"
            value={stats?.total_sessions || 0}
            icon={<SessionIcon />}
            color="#2563eb"
          />
        </Box>
        <Box sx={{ width: { xs: 'calc(50% - 8px)', md: 'calc(25% - 18px)' } }}>
          <StatCard
            title="Active"
            value={stats?.active_sessions || 0}
            icon={<RunningIcon />}
            color="#0891b2"
          />
        </Box>
        <Box sx={{ width: { xs: 'calc(50% - 8px)', md: 'calc(25% - 18px)' } }}>
          <StatCard
            title="Completed"
            value={stats?.completed_sessions || 0}
            icon={<CheckIcon />}
            color="#10b981"
          />
        </Box>
        <Box sx={{ width: { xs: 'calc(50% - 8px)', md: 'calc(25% - 18px)' } }}>
          <StatCard
            title="Failed"
            value={stats?.failed_sessions || 0}
            icon={<ErrorIcon />}
            color="#ef4444"
          />
        </Box>
      </Box>

      {/* Second row */}
      <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 3, mb: 3 }}>
        <Box sx={{ width: { xs: '100%', md: 'calc(33.333% - 16px)' } }}>
          <Card sx={{ height: '100%' }}>
            <CardContent sx={{ p: 2 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1.5 }}>
                <TimeIcon color="primary" fontSize="small" />
                <Typography variant="subtitle1" fontWeight={600}>
                  Uptime
                </Typography>
              </Box>
              <Typography variant="h5" fontWeight={700} sx={{ fontSize: { xs: '1.5rem', sm: '2rem' } }}>
                {stats?.uptime ? formatDuration(stats.uptime) : '-'}
              </Typography>
              <Typography variant="caption" color="text.secondary">
                Since last restart
              </Typography>
            </CardContent>
          </Card>
        </Box>

        <Box sx={{ width: { xs: '100%', md: 'calc(33.333% - 16px)' } }}>
          <Card sx={{ height: '100%' }}>
            <CardContent sx={{ p: 2 }}>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1.5 }}>
                <SecurityIcon color="warning" fontSize="small" />
                <Typography variant="subtitle1" fontWeight={600}>
                  Rate Limit
                </Typography>
              </Box>
              <Typography variant="h5" fontWeight={700} sx={{ fontSize: { xs: '1.5rem', sm: '2rem' } }}>
                <>{stats?.rate_limit_stats?.currently_blocked || 0}</>
              </Typography>
              <Typography variant="caption" color="text.secondary">
                IPs currently blocked
              </Typography>
            </CardContent>
          </Card>
        </Box>

        <Box sx={{ width: { xs: '100%', md: 'calc(33.333% - 16px)' } }}>
          <Card sx={{ height: '100%' }}>
            <CardContent sx={{ p: 2 }}>
              <Typography variant="subtitle1" fontWeight={600} gutterBottom>
                Recent Actions (24h)
              </Typography>
              <List dense sx={{ py: 0 }}>
                <ListItem sx={{ py: 0.5 }}>
                  <ListItemIcon sx={{ minWidth: 32 }}>
                    <RunningIcon fontSize="small" />
                  </ListItemIcon>
                  <ListItemText
                    primary="Active"
                    secondary={stats?.recent_actions?.running?.toString() || '0'}
                    primaryTypographyProps={{ variant: 'body2' }}
                    secondaryTypographyProps={{ variant: 'caption' }}
                  />
                </ListItem>
                <Divider />
                <ListItem sx={{ py: 0.5 }}>
                  <ListItemIcon sx={{ minWidth: 32 }}>
                    <CheckIcon fontSize="small" color="success" />
                  </ListItemIcon>
                  <ListItemText
                    primary="Completed"
                    secondary={stats?.recent_actions?.completed?.toString() || '0'}
                    primaryTypographyProps={{ variant: 'body2' }}
                    secondaryTypographyProps={{ variant: 'caption' }}
                  />
                </ListItem>
                <Divider />
                <ListItem sx={{ py: 0.5 }}>
                  <ListItemIcon sx={{ minWidth: 32 }}>
                    <ErrorIcon fontSize="small" color="error" />
                  </ListItemIcon>
                  <ListItemText
                    primary="Failed"
                    secondary={stats?.recent_actions?.failed?.toString() || '0'}
                    primaryTypographyProps={{ variant: 'body2' }}
                    secondaryTypographyProps={{ variant: 'caption' }}
                  />
                </ListItem>
                <Divider />
                <ListItem sx={{ py: 0.5 }}>
                  <ListItemIcon sx={{ minWidth: 32 }}>
                    <CancelIcon fontSize="small" />
                  </ListItemIcon>
                  <ListItemText
                    primary="Closed"
                    secondary={stats?.recent_actions?.closed?.toString() || '0'}
                    primaryTypographyProps={{ variant: 'body2' }}
                    secondaryTypographyProps={{ variant: 'caption' }}
                  />
                </ListItem>
              </List>
            </CardContent>
          </Card>
        </Box>
      </Box>
    </Box>
  );
};
