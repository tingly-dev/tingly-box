//go:build kong

package main

import "fmt"

// MatrixKong runs protocol validation matrix tests
type MatrixKong struct {
	Scenarios  []string `kong:"flag,name='scenario',short='s',help='Test scenarios'"`
	Sources    []string `kong:"flag,name='source',help='Source protocols'"`
	Targets    []string `kong:"flag,name='target',help='Target protocols'"`
	Streaming  bool     `kong:"flag,name='streaming',help='Run only streaming tests'"`
	NonStream  bool     `kong:"flag,name='non-stream',help='Run only non-streaming tests'"`
	JsonOutput bool     `kong:"flag,name='json',help='JSON output'"`
	Verbose    []bool   `kong:"flag,name='verbose',short='v',help='Verbose level'"`
	RecordDir  string   `kong:"flag,name='record-dir',help='Recording directory'"`
	ServerMode string   `kong:"flag,name='server-mode',help='Server reuse mode'"`
	BatchCount int      `kong:"flag,name='batch',help='Batch count'"`
}

func (m *MatrixKong) Run() error {
	cmd := newMatrixCommand()
	args := buildArgsFromMatrix(m)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func buildArgsFromMatrix(m *MatrixKong) []string {
	args := []string{}
	for _, s := range m.Scenarios {
		args = append(args, "--scenario", s)
	}
	for _, s := range m.Sources {
		args = append(args, "--source", s)
	}
	for _, t := range m.Targets {
		args = append(args, "--target", t)
	}
	if m.Streaming {
		args = append(args, "--streaming")
	}
	if m.NonStream {
		args = append(args, "--non-stream")
	}
	if m.JsonOutput {
		args = append(args, "--json")
	}
	for i := 0; i < len(m.Verbose); i++ {
		args = append(args, "-v")
	}
	if m.RecordDir != "" {
		args = append(args, "--record-dir", m.RecordDir)
	}
	if m.ServerMode != "" {
		args = append(args, "--server-mode", m.ServerMode)
	}
	if m.BatchCount > 0 {
		args = append(args, "--batch", fmt.Sprintf("%d", m.BatchCount))
	}
	return args
}

// AgentKong runs agent e2e tests
type AgentKong struct {
	Mock      bool   `kong:"flag,name='mock',help='Use virtual upstream provider'"`
	Config    string `kong:"flag,name='config',help='Config file for real providers'"`
	Timeout   int    `kong:"flag,name='timeout',short='t',help='Timeout in seconds'"`
	AgentType string `kong:"arg,optional,help='Agent type (claude, codex, opencode, batch)'"`
}

func (a *AgentKong) Run() error {
	cmd := newAgentCommand()
	args := []string{}
	if a.AgentType != "" {
		args = append(args, a.AgentType)
	}
	if a.Mock {
		args = append(args, "--mock")
	}
	if a.Config != "" {
		args = append(args, "--config", a.Config)
	}
	if a.Timeout > 0 {
		args = append(args, "--timeout", fmt.Sprintf("%d", a.Timeout))
	}
	cmd.SetArgs(args)
	return cmd.Execute()
}

// ProviderKong runs real provider API tests
type ProviderKong struct {
	Test ProviderTestKong `kong:"cmd,help='Run provider tests'"`
	List ProviderListKong `kong:"cmd,help='List providers'"`
}

func (p *ProviderKong) Run() error {
	return p.List.Run()
}

// ProviderTestKong runs provider tests
type ProviderTestKong struct {
	Provider  string   `kong:"arg,optional,help='Provider name or UUID'"`
	Scenarios []string `kong:"flag,name='scenario',help='Test scenarios'"`
}

func (p *ProviderTestKong) Run() error {
	cmd := newProviderCommand()
	args := []string{"test"}
	if p.Provider != "" {
		args = append(args, p.Provider)
	}
	for _, s := range p.Scenarios {
		args = append(args, "--scenario", s)
	}
	cmd.SetArgs(args)
	return cmd.Execute()
}

// ProviderListKong lists providers
type ProviderListKong struct{}

func (p *ProviderListKong) Run() error {
	cmd := newProviderCommand()
	cmd.SetArgs([]string{"list"})
	return cmd.Execute()
}

// InitConfigKong creates config file template
type InitConfigKong struct {
	Output string `kong:"flag,name='output',short='o',help='Output file path'"`
}

func (i *InitConfigKong) Run() error {
	return runInitConfig(i.Output)
}

// VersionKong shows version
type VersionKong struct{}

func (v *VersionKong) Run() error {
	fmt.Printf("Tingly-Box Protocol Validation Harness\n")
	fmt.Printf("Version:   %s\n", version)
	fmt.Printf("Commit:    %s\n", gitCommit)
	fmt.Printf("Built:     %s\n", buildTime)
	return nil
}
