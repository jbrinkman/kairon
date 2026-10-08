package eval

import (
	"fmt"
	"os"

	"github.com/jbrinkman/kairon/internal/eval/sandbox"
)

const (
	// networkUnrestricted is the recorded containment.network value. Kairon
	// makes no network containment guarantee; this is NOT the container
	// runtime's NetworkMode (see docs/evaluation.md).
	networkUnrestricted = "unrestricted"

	// fakeGHInstalled is whether container runs get the fake gh. It is the
	// switch buildContainerMounts honours (the fake gh bin directory is
	// mounted only when it is true) and the value recorded as containment.fake_gh.
	fakeGHInstalled = true
)

// containmentFor returns the containment that applies to a container run of
// agent. Each fact comes from the same source that enforces it. If the trust
// set cannot be resolved the container call fails closed too, so tool_trust
// is recorded as [] and a warning is printed; stamping never fails the run.
func containmentFor(agent string) Containment {
	toolTrust := []string{}
	trust, err := resolveTrustSet(agent)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  Warning: cannot resolve tool-trust set for %s; recording tool_trust [] in containment: %v\n", agent, err)
	} else if names := trust.Names(); names != nil {
		toolTrust = names
	}
	return Containment{
		ToolTrust:  toolTrust,
		FakeGH:     fakeGHInstalled,
		ReadOnlyFS: sandbox.ReadonlyRootfs,
		Network:    networkUnrestricted,
	}
}

// applyRunContext stamps r with everything that describes how it was run:
// the pinned provenance (shared by native and container runs), the execution
// mode and, for a container run, the containment. A nil cfg is a native run:
// Sandbox is false and any Containment (e.g. from a resumed file) is cleared.
// It never changes a provenance field beyond what pins.applyTo sets.
func applyRunContext(r *AgentResult, cConfig *ContainerConfig) {
	cfg.pins.applyTo(r)
	isContainer := cConfig != nil
	r.Sandbox = &isContainer
	if isContainer {
		c := containmentFor(r.Agent)
		r.Containment = &c
		return
	}
	r.Containment = nil
}
