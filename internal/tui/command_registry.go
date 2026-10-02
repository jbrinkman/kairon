package tui

import (
	"sort"
	"strconv"
	"strings"

	"github.com/jbrinkman/kairon/internal/agent"
)

// Command represents a command with its metadata for autocomplete
type Command struct {
	Name        string   // Primary command name
	Description string   // Help text
	Subcommands []string // Available subcommands
	HasArgs     bool     // Whether command accepts arguments
	ArgPattern  string   // Pattern for arguments (e.g., "<issue>", "[desc]")
}

// CommandRegistry manages all available commands for autocomplete
type CommandRegistry struct {
	commands          map[string]*Command
	flattenedCommands []string // Compound commands as single units
	agentManager      *agent.Manager
}

// NewCommandRegistry creates a new command registry with all REPL commands
func NewCommandRegistry(agentManager *agent.Manager) *CommandRegistry {
	registry := &CommandRegistry{
		commands:          make(map[string]*Command),
		flattenedCommands: []string{},
		agentManager:      agentManager,
	}

	// Register all commands
	registry.register(&Command{
		Name:        "watch",
		Description: "Start or stop the issue watcher",
		Subcommands: []string{"start", "stop"},
	})

	registry.register(&Command{
		Name:        "status",
		Description: "Show system status and running agents",
	})

	registry.register(&Command{
		Name:        "stop",
		Description: "Stop agent for specific issue number",
		HasArgs:     true,
		ArgPattern:  "<issue>",
	})

	registry.register(&Command{
		Name:        "plan",
		Description: "Start interactive planning session",
		Subcommands: []string{"classic"},
		HasArgs:     true,
		ArgPattern:  "[desc]",
	})

	registry.register(&Command{
		Name:        "theme",
		Description: "Show current theme or switch to new theme",
		HasArgs:     true,
		ArgPattern:  "[name]",
	})

	registry.register(&Command{
		Name:        "about",
		Description: "Show version information and check for updates",
	})

	registry.register(&Command{
		Name:        "exit",
		Description: "Exit the application",
	})

	registry.register(&Command{
		Name:        "help",
		Description: "Show available commands",
	})

	registry.register(&Command{
		Name:        "logs",
		Description: "View incident logs",
	})

	registry.register(&Command{
		Name:        "log",
		Description: "Open log viewer with optional level and buffer size",
		HasArgs:     true,
		ArgPattern:  "[level] [size]",
	})

	// Build flattened command list
	registry.buildFlattenedCommands()

	return registry
}

// register adds a command to the registry
func (r *CommandRegistry) register(cmd *Command) {
	r.commands[cmd.Name] = cmd
}

// buildFlattenedCommands creates a list of all commands including compound ones
func (r *CommandRegistry) buildFlattenedCommands() {
	r.flattenedCommands = []string{}

	for _, cmd := range r.commands {
		// Add base command
		r.flattenedCommands = append(r.flattenedCommands, cmd.Name)

		// Add compound commands (command + subcommand)
		for _, sub := range cmd.Subcommands {
			r.flattenedCommands = append(r.flattenedCommands, cmd.Name+" "+sub)
		}
	}
}

// GetCommand returns a command by name
func (r *CommandRegistry) GetCommand(name string) (*Command, bool) {
	cmd, exists := r.commands[name]
	return cmd, exists
}

// GetAllCommands returns all registered commands
func (r *CommandRegistry) GetAllCommands() []*Command {
	commands := make([]*Command, 0, len(r.commands))
	for _, cmd := range r.commands {
		commands = append(commands, cmd)
	}
	return commands
}

// FilterCommands returns commands that match the input prefix
func (r *CommandRegistry) FilterCommands(input string) []*Command {
	if input == "" {
		return r.GetAllCommands()
	}

	parts := strings.Fields(input)
	if len(parts) == 0 {
		return r.GetAllCommands()
	}

	prefix := strings.ToLower(parts[0])
	matches := make([]*Command, 0)

	for _, cmd := range r.commands {
		if strings.HasPrefix(strings.ToLower(cmd.Name), prefix) {
			matches = append(matches, cmd)
		}
	}

	return matches
}

// GetFlattenedMatches returns flattened command strings that match the input prefix
func (r *CommandRegistry) GetFlattenedMatches(input string) []string {
	if input == "" {
		return r.flattenedCommands
	}

	inputLower := strings.ToLower(input)
	matches := []string{}

	// Handle dynamic stop commands
	if strings.HasPrefix(inputLower, "stop") {
		stopMatches := r.generateStopCommands(input)
		matches = append(matches, stopMatches...)
	}

	// Add other flattened commands
	for _, cmd := range r.flattenedCommands {
		cmdLower := strings.ToLower(cmd)
		if strings.HasPrefix(cmdLower, inputLower) && !strings.HasPrefix(cmdLower, "stop") {
			matches = append(matches, cmd)
		}
	}

	return matches
}

// generateStopCommands creates contextual stop commands based on running agents
func (r *CommandRegistry) generateStopCommands(input string) []string {
	if r.agentManager == nil {
		return []string{}
	}

	agents := r.agentManager.List()
	var runningAgents []*agent.Agent
	for _, ag := range agents {
		if ag.Status == agent.StatusRunning {
			runningAgents = append(runningAgents, ag)
		}
	}

	if len(runningAgents) == 0 {
		return []string{}
	}

	inputLower := strings.ToLower(input)

	// Sort agents by issue number for consistent ordering
	sort.Slice(runningAgents, func(i, j int) bool {
		return runningAgents[i].IssueNumber < runningAgents[j].IssueNumber
	})

	if len(runningAgents) <= 10 {
		// Generate specific stop commands
		var matches []string
		for _, ag := range runningAgents {
			stopCmd := "stop " + strconv.Itoa(ag.IssueNumber)
			if strings.HasPrefix(strings.ToLower(stopCmd), inputLower) {
				matches = append(matches, stopCmd)
			}
		}
		return matches
	} else {
		// Generate template command
		templateCmd := "stop <issue number>"
		if strings.HasPrefix(strings.ToLower(templateCmd), inputLower) {
			return []string{templateCmd}
		}
		return []string{}
	}
}

// GetSubcommands returns subcommands for a given command and input
func (r *CommandRegistry) GetSubcommands(input string) []string {
	parts := strings.Fields(input)
	if len(parts) < 1 {
		return nil
	}

	cmdName := strings.ToLower(parts[0])
	cmd, exists := r.commands[cmdName]
	if !exists || len(cmd.Subcommands) == 0 {
		return nil
	}

	// If we have exactly one part, return all subcommands
	if len(parts) == 1 {
		return cmd.Subcommands
	}

	// If we have two parts, filter subcommands by prefix
	if len(parts) == 2 {
		subPrefix := strings.ToLower(parts[1])
		matches := make([]string, 0)
		for _, sub := range cmd.Subcommands {
			if strings.HasPrefix(strings.ToLower(sub), subPrefix) {
				matches = append(matches, sub)
			}
		}
		return matches
	}

	return nil
}

// GetBestMatch returns the best completion match for the input
func (r *CommandRegistry) GetBestMatch(input string) string {
	if input == "" {
		return ""
	}

	parts := strings.Fields(input)
	if len(parts) == 0 {
		return ""
	}

	// Check for command match
	commands := r.FilterCommands(input)
	if len(commands) > 0 {
		cmdName := commands[0].Name

		// If input is just the command name, return it
		if len(parts) == 1 {
			return cmdName
		}

		// Check for subcommand match
		subcommands := r.GetSubcommands(input)
		if len(subcommands) > 0 {
			return cmdName + " " + subcommands[0]
		}

		return cmdName
	}

	return ""
}

// IsValidCommand checks if the input represents a valid command
func (r *CommandRegistry) IsValidCommand(input string) bool {
	input = strings.TrimSpace(input)
	if input == "" {
		return false
	}

	parts := strings.Fields(input)
	if len(parts) == 0 {
		return false
	}

	cmdName := strings.ToLower(parts[0])
	cmd, exists := r.commands[cmdName]
	if !exists {
		return false
	}

	// Check subcommand if present
	if len(parts) > 1 && len(cmd.Subcommands) > 0 {
		subCmd := strings.ToLower(parts[1])
		for _, validSub := range cmd.Subcommands {
			if strings.ToLower(validSub) == subCmd {
				return true
			}
		}
		// Allow fallback if the command accepts normal arguments
		if cmd.HasArgs {
			return true
		}
		return false
	}

	return true
}
