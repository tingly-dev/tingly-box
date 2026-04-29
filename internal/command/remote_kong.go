//go:build !legacy

package command

// RemoteCmdKong manages remote control. The hidden Default subcommand is
// marked default so `tingly-box remote` (no further args) lists sessions,
// matching the legacy behavior of showing help/list.
type RemoteCmdKong struct {
	Default RemoteListCmdKong   `kong:"cmd,name='default',default='1',hidden,help='List remote sessions (default)'"`
	List    RemoteListCmdKong   `kong:"cmd,help='List remote sessions'"`
	Start   RemoteStartCmdKong  `kong:"cmd,help='Start a remote session'"`
	Config  RemoteConfigCmdKong `kong:"cmd,help='Configure remote settings'"`
	Add     RemoteAddCmdKong    `kong:"cmd,help='Add a new bot configuration'"`
}

// RemoteListCmdKong lists remote sessions
type RemoteListCmdKong struct{}

func (r *RemoteListCmdKong) Run(appManager *AppManager) error {
	cmd := RemoteCommand(appManager)
	cmd.SetArgs([]string{"list"})
	return cmd.Execute()
}

// RemoteStartCmdKong starts a remote session
type RemoteStartCmdKong struct {
	UUID     string `kong:"arg,optional,help='Bot UUID (interactive selection if omitted)'"`
	DataPath string `kong:"flag,name='data-path',help='Data directory for bot state'"`
	Provider string `kong:"flag,name='provider',help='Provider UUID for smartguide'"`
	Model    string `kong:"flag,name='model',help='Model name for smartguide'"`
	Force    bool   `kong:"flag,name='force',help='Skip provider validation and force start'"`
}

func (r *RemoteStartCmdKong) Run(appManager *AppManager) error {
	cmd := RemoteCommand(appManager)
	args := []string{"start"}
	if r.UUID != "" {
		args = append(args, r.UUID)
	}
	if r.DataPath != "" {
		args = append(args, "--data-path", r.DataPath)
	}
	if r.Provider != "" {
		args = append(args, "--provider", r.Provider)
	}
	if r.Model != "" {
		args = append(args, "--model", r.Model)
	}
	if r.Force {
		args = append(args, "--force")
	}
	cmd.SetArgs(args)
	return cmd.Execute()
}

// RemoteConfigCmdKong configures remote settings
type RemoteConfigCmdKong struct {
	UUID     string `kong:"arg,optional,help='Bot UUID (interactive selection if omitted)'"`
	Show     bool   `kong:"flag,name='show',help='Show current configuration'"`
	Provider string `kong:"flag,name='provider',help='Provider UUID for smartguide'"`
	Model    string `kong:"flag,name='model',help='Model name for smartguide'"`
}

func (r *RemoteConfigCmdKong) Run(appManager *AppManager) error {
	cmd := RemoteCommand(appManager)
	args := []string{"config"}
	if r.UUID != "" {
		args = append(args, r.UUID)
	}
	if r.Show {
		args = append(args, "--show")
	}
	if r.Provider != "" {
		args = append(args, "--provider", r.Provider)
	}
	if r.Model != "" {
		args = append(args, "--model", r.Model)
	}
	cmd.SetArgs(args)
	return cmd.Execute()
}

// RemoteAddCmdKong adds a new bot configuration (interactive).
type RemoteAddCmdKong struct{}

func (r *RemoteAddCmdKong) Run(appManager *AppManager) error {
	cmd := RemoteCommand(appManager)
	cmd.SetArgs([]string{"add"})
	return cmd.Execute()
}
