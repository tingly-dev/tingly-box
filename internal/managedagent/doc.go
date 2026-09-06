// Package managedagent is the domain layer for managed agent sessions: pick a
// git Source, pick an Environment, and let a coding agent work in an isolated
// Workspace while the user steers it from the web UI or an IM bot.
//
// The package is deliberately a leaf: domain types, store interfaces, and the
// Service that enforces invariants. Persistence lives in internal/db (SQLite
// index + append-only event log), the HTTP surface in
// internal/server/module/managedagent, and execution (agentboot + workspace
// provisioning) is wired in through the Launcher seam so this package never
// imports a process runtime.
//
// Vocabulary is fixed by .design/managed-agent.md §3 and does not reuse the
// bot/channel/scenario/profile words already taken by the remote subsystem.
package managedagent
