//go:build kong

package command

// RemoteCmdKong manages remote control
type RemoteCmdKong struct {
	List   RemoteListCmdKong   `kong:"cmd,help='List remote sessions'"`
	Start  RemoteStartCmdKong  `kong:"cmd,help='Start a remote session'"`
	Config RemoteConfigCmdKong `kong:"cmd,help='Configure remote settings'"`
}

func (r *RemoteCmdKong) Run(appManager *AppManager) error {
	cmd := RemoteCommand(appManager)
	cmd.SetArgs([]string{"list"})
	return cmd.Execute()
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
	Name string `kong:"arg,optional,help='Session name'"`
}

func (r *RemoteStartCmdKong) Run(appManager *AppManager) error {
	cmd := RemoteCommand(appManager)
	args := []string{"start"}
	if r.Name != "" {
		args = append(args, r.Name)
	}
	cmd.SetArgs(args)
	return cmd.Execute()
}

// RemoteConfigCmdKong configures remote settings
type RemoteConfigCmdKong struct{}

func (r *RemoteConfigCmdKong) Run(appManager *AppManager) error {
	cmd := RemoteCommand(appManager)
	cmd.SetArgs([]string{"config"})
	return cmd.Execute()
}
