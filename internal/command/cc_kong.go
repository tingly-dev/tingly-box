package command

// CCmdKong launches Claude Code with passthrough mode
// Kong's passthrough mode requires at least one positional arg
type CCmdKong struct {
	Args []string `kong:"arg,optional,passthrough"`
}

func (c *CCmdKong) Run(appManager *AppManager) error {
	profile, claudeArgs, err := parseCCFlags(c.Args)
	if err != nil {
		return err
	}
	return runCC(appManager, profile, claudeArgs)
}
