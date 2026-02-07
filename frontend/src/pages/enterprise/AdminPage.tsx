import React, { useState, useEffect } from 'react';
import {
  Box,
  Container,
  Paper,
  Typography,
  Tabs,
  Tab,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Button,
  Chip,
  IconButton,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Pagination,
  Alert,
  CircularProgress,
} from '@mui/material';
import {
  Edit as EditIcon,
  Delete as DeleteIcon,
  LockReset as LockResetIcon,
  CheckCircle as CheckCircleIcon,
  Cancel as CancelIcon,
} from '@mui/icons-material';
import { useNavigate } from 'react-router-dom';
import { useEnterpriseAuth } from '../../contexts/EnterpriseAuthContext';
import { enterpriseAPI } from '../../services/enterprise/api';
import type { User, Role } from '../../types/enterprise';

interface TabPanelProps {
  children?: React.ReactNode;
  index: number;
  value: number;
}

function TabPanel(props: TabPanelProps) {
  const { children, value, index, ...other } = props;
  return (
    <div role="tabpanel" hidden={value !== index} {...other}>
      {value === index && <Box sx={{ py: 3 }}>{children}</Box>}
    </div>
  );
}

const EnterpriseAdminPage: React.FC = () => {
  const navigate = useNavigate();
  const { user, hasRole, logout } = useEnterpriseAuth();
  const [tabValue, setTabValue] = useState(0);

  // Users state
  const [users, setUsers] = useState<User[]>([]);
  const [usersPage, setUsersPage] = useState(1);
  const [usersTotal, setUsersTotal] = useState(0);
  const [usersLoading, setUsersLoading] = useState(false);

  // Dialog states
  const [createUserOpen, setCreateUserOpen] = useState(false);
  const [editUserOpen, setEditUserOpen] = useState(false);
  const [selectedUser, setSelectedUser] = useState<User | null>(null);
  const [showPassword, setShowPassword] = useState('');

  // Form states
  const [username, setUsername] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [fullName, setFullName] = useState('');
  const [role, setRole] = useState<Role>('user');

  // Message state
  const [message, setMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null);

  useEffect(() => {
    if (!user) {
      navigate('/enterprise/login');
      return;
    }

    if (!hasRole('admin')) {
      navigate('/');
      return;
    }

    loadUsers();
  }, [user, navigate, hasRole]);

  const loadUsers = async (page: number = 1) => {
    setUsersLoading(true);
    try {
      const response = await enterpriseAPI.listUsers(page, 20);
      setUsers(response.users);
      setUsersTotal(response.total);
      setUsersPage(page);
    } catch (error) {
      showMessage('error', 'Failed to load users');
    } finally {
      setUsersLoading(false);
    }
  };

  const handleCreateUser = async () => {
    try {
      await enterpriseAPI.createUser({
        username,
        email,
        password,
        full_name: fullName,
        role,
      });
      showMessage('success', 'User created successfully');
      setCreateUserOpen(false);
      resetForm();
      loadUsers();
    } catch (error: any) {
      showMessage('error', error.response?.data?.error || 'Failed to create user');
    }
  };

  const handleUpdateUser = async () => {
    if (!selectedUser) return;

    try {
      await enterpriseAPI.updateUser(selectedUser.id, {
        full_name: fullName,
        role,
      });
      showMessage('success', 'User updated successfully');
      setEditUserOpen(false);
      setSelectedUser(null);
      resetForm();
      loadUsers();
    } catch (error: any) {
      showMessage('error', error.response?.data?.error || 'Failed to update user');
    }
  };

  const handleDeleteUser = async (userId: number) => {
    if (!confirm('Are you sure you want to delete this user?')) return;

    try {
      await enterpriseAPI.deleteUser(userId);
      showMessage('success', 'User deleted successfully');
      loadUsers();
    } catch (error: any) {
      showMessage('error', error.response?.data?.error || 'Failed to delete user');
    }
  };

  const handleToggleActive = async (userId: number, isActive: boolean) => {
    try {
      if (isActive) {
        await enterpriseAPI.deactivateUser(userId);
        showMessage('success', 'User deactivated');
      } else {
        await enterpriseAPI.activateUser(userId);
        showMessage('success', 'User activated');
      }
      loadUsers();
    } catch (error: any) {
      showMessage('error', error.response?.data?.error || 'Failed to update user status');
    }
  };

  const handleResetPassword = async (userId: number) => {
    try {
      const response = await enterpriseAPI.resetPassword(userId);
      setShowPassword(response.new_password);
      setEditUserOpen(true);
      setSelectedUser(users.find((u) => u.id === userId) || null);
    } catch (error: any) {
      showMessage('error', error.response?.data?.error || 'Failed to reset password');
    }
  };

  const openEditDialog = (user: User) => {
    setSelectedUser(user);
    setUsername(user.username);
    setEmail(user.email);
    setFullName(user.full_name);
    setRole(user.role);
    setEditUserOpen(true);
  };

  const resetForm = () => {
    setUsername('');
    setEmail('');
    setPassword('');
    setFullName('');
    setRole('user');
    setShowPassword('');
  };

  const showMessage = (type: 'success' | 'error', text: string) => {
    setMessage({ type, text });
    setTimeout(() => setMessage(null), 5000);
  };

  const getRoleColor = (role: Role) => {
    switch (role) {
      case 'admin':
        return 'error';
      case 'user':
        return 'primary';
      case 'readonly':
        return 'default';
      default:
        return 'default';
    }
  };

  if (!user) {
    return null;
  }

  return (
    <Container maxWidth="xl" sx={{ mt: 4, mb: 4 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
        <Typography variant="h4" component="h1">
          Enterprise Admin Panel
        </Typography>
        <Box sx={{ display: 'flex', gap: 2, alignItems: 'center' }}>
          <Typography variant="body1" color="text.secondary">
            Logged in as <strong>{user.username}</strong> ({user.role})
          </Typography>
          <Button variant="outlined" onClick={logout}>
            Logout
          </Button>
        </Box>
      </Box>

      {message && (
        <Alert severity={message.type} sx={{ mb: 3 }} onClose={() => setMessage(null)}>
          {message.text}
        </Alert>
      )}

      <Paper sx={{ width: '100%' }}>
        <Tabs
          value={tabValue}
          onChange={(_, newValue) => setTabValue(newValue)}
          indicatorColor="primary"
          textColor="primary"
        >
          <Tab label="Users" />
          <Tab label="Tokens" />
          <Tab label="Audit Logs" />
        </Tabs>

        <TabPanel value={tabValue} index={0}>
          <Box sx={{ display: 'flex', justifyContent: 'flex-end', mb: 2 }}>
            <Button
              variant="contained"
              onClick={() => {
                resetForm();
                setCreateUserOpen(true);
              }}
            >
              Create User
            </Button>
          </Box>

          {usersLoading ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', py: 3 }}>
              <CircularProgress />
            </Box>
          ) : (
            <>
              <TableContainer>
                <Table>
                  <TableHead>
                    <TableRow>
                      <TableCell>Username</TableCell>
                      <TableCell>Email</TableCell>
                      <TableCell>Full Name</TableCell>
                      <TableCell>Role</TableCell>
                      <TableCell>Status</TableCell>
                      <TableCell>Last Login</TableCell>
                      <TableCell>Actions</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {users.map((u) => (
                      <TableRow key={u.id}>
                        <TableCell>{u.username}</TableCell>
                        <TableCell>{u.email}</TableCell>
                        <TableCell>{u.full_name || '-'}</TableCell>
                        <TableCell>
                          <Chip label={u.role} color={getRoleColor(u.role) as any} size="small" />
                        </TableCell>
                        <TableCell>
                          <Chip
                            label={u.is_active ? 'Active' : 'Inactive'}
                            color={u.is_active ? 'success' : 'default'}
                            size="small"
                          />
                        </TableCell>
                        <TableCell>
                          {u.last_login_at
                            ? new Date(u.last_login_at).toLocaleString()
                            : 'Never'}
                        </TableCell>
                        <TableCell>
                          <IconButton
                            size="small"
                            onClick={() => openEditDialog(u)}
                            title="Edit"
                          >
                            <EditIcon fontSize="small" />
                          </IconButton>
                          {u.id !== user.id && (
                            <>
                              <IconButton
                                size="small"
                                onClick={() => handleToggleActive(u.id, u.is_active)}
                                title={u.is_active ? 'Deactivate' : 'Activate'}
                              >
                                {u.is_active ? (
                                  <CancelIcon fontSize="small" />
                                ) : (
                                  <CheckCircleIcon fontSize="small" />
                                )}
                              </IconButton>
                              <IconButton
                                size="small"
                                onClick={() => handleResetPassword(u.id)}
                                title="Reset Password"
                              >
                                <LockResetIcon fontSize="small" />
                              </IconButton>
                              <IconButton
                                size="small"
                                onClick={() => handleDeleteUser(u.id)}
                                title="Delete"
                              >
                                <DeleteIcon fontSize="small" />
                              </IconButton>
                            </>
                          )}
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </TableContainer>

              <Box sx={{ display: 'flex', justifyContent: 'center', mt: 3 }}>
                <Pagination
                  count={Math.ceil(usersTotal / 20)}
                  page={usersPage}
                  onChange={(_, page) => loadUsers(page)}
                />
              </Box>
            </>
          )}
        </TabPanel>

        <TabPanel value={tabValue} index={1}>
          <Typography variant="body1" color="text.secondary">
            Token management coming soon...
          </Typography>
        </TabPanel>

        <TabPanel value={tabValue} index={2}>
          <Typography variant="body1" color="text.secondary">
            Audit logs coming soon...
          </Typography>
        </TabPanel>
      </Paper>

      {/* Create User Dialog */}
      <Dialog open={createUserOpen} onClose={() => setCreateUserOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>Create User</DialogTitle>
        <DialogContent>
          <TextField
            autoFocus
            margin="dense"
            label="Username"
            fullWidth
            variant="outlined"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            sx={{ mb: 2 }}
          />
          <TextField
            margin="dense"
            label="Email"
            type="email"
            fullWidth
            variant="outlined"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            sx={{ mb: 2 }}
          />
          <TextField
            margin="dense"
            label="Password"
            type="password"
            fullWidth
            variant="outlined"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            sx={{ mb: 2 }}
          />
          <TextField
            margin="dense"
            label="Full Name"
            fullWidth
            variant="outlined"
            value={fullName}
            onChange={(e) => setFullName(e.target.value)}
            sx={{ mb: 2 }}
          />
          <FormControl fullWidth variant="outlined">
            <InputLabel>Role</InputLabel>
            <Select
              value={role}
              onChange={(e) => setRole(e.target.value as Role)}
              label="Role"
            >
              <MenuItem value="user">User</MenuItem>
              <MenuItem value="admin">Admin</MenuItem>
              <MenuItem value="readonly">Read Only</MenuItem>
            </Select>
          </FormControl>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setCreateUserOpen(false)}>Cancel</Button>
          <Button onClick={handleCreateUser} variant="contained">
            Create
          </Button>
        </DialogActions>
      </Dialog>

      {/* Edit User Dialog */}
      <Dialog open={editUserOpen} onClose={() => setEditUserOpen(false)} maxWidth="sm" fullWidth>
        <DialogTitle>
          {showPassword ? 'Password Reset' : 'Edit User'}
        </DialogTitle>
        <DialogContent>
          {showPassword ? (
            <Alert severity="info" sx={{ mt: 2 }}>
              <Typography variant="body2">
                New password for <strong>{selectedUser?.username}</strong>:
              </Typography>
              <Typography
                variant="h6"
                sx={{ mt: 1, fontFamily: 'monospace', wordBreak: 'break-all' }}
              >
                {showPassword}
              </Typography>
              <Typography variant="caption" display="block" sx={{ mt: 1 }}>
                Please save this password. It will not be shown again.
              </Typography>
            </Alert>
          ) : (
            <>
              <TextField
                margin="dense"
                label="Username"
                fullWidth
                variant="outlined"
                value={username}
                disabled
                sx={{ mb: 2 }}
              />
              <TextField
                margin="dense"
                label="Email"
                type="email"
                fullWidth
                variant="outlined"
                value={email}
                disabled
                sx={{ mb: 2 }}
              />
              <TextField
                margin="dense"
                label="Full Name"
                fullWidth
                variant="outlined"
                value={fullName}
                onChange={(e) => setFullName(e.target.value)}
                sx={{ mb: 2 }}
              />
              <FormControl fullWidth variant="outlined">
                <InputLabel>Role</InputLabel>
                <Select
                  value={role}
                  onChange={(e) => setRole(e.target.value as Role)}
                  label="Role"
                  disabled={selectedUser?.id === user.id}
                >
                  <MenuItem value="user">User</MenuItem>
                  <MenuItem value="admin">Admin</MenuItem>
                  <MenuItem value="readonly">Read Only</MenuItem>
                </Select>
              </FormControl>
            </>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={() => { setEditUserOpen(false); setShowPassword(''); }}>
            {showPassword ? 'Close' : 'Cancel'}
          </Button>
          {!showPassword && (
            <Button onClick={handleUpdateUser} variant="contained">
              Save
            </Button>
          )}
        </DialogActions>
      </Dialog>
    </Container>
  );
};

export default EnterpriseAdminPage;
