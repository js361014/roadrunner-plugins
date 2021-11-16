package informer

import (
	"context"

	"github.com/spiral/roadrunner-plugins/v2/api/jobs"
	"github.com/spiral/roadrunner/v2/state/process"
)

/*
Informer plugin should not receive any other plugin in the Init or via Collects
Because Availabler implementation should present in every plugin
*/

// Statistic interfaces ==============

// Informer used to get workers from particular plugin or set of plugins
type Informer interface {
	Workers() []*process.State
}

// JobsStat interface provide statistic for the jobs plugin
type JobsStat interface {
	// JobsState returns slice with the attached drivers information
	JobsState(ctx context.Context) ([]*jobs.State, error)
}

// Statistic interfaces end ============

// Availabler interface should be implemented by every plugin which wish to report to the PHP worker that it available in the RR runtime
type Availabler interface {
	// Available method needed to collect all plugins which are available in the runtime.
	Available()
}
