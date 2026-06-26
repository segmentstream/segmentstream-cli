package projectcheck

import "github.com/segmentstream/segmentstream-cli/cli/internal/cliresult"

const (
	statusSatisfied = "satisfied"
	statusMissing   = "missing"
	statusPending   = "pending"
	statusInvalid   = "invalid"
	statusUntested  = "untested"
)

type stageID string

const (
	stagePrerequisites   stageID = "prerequisites"
	stageWarehouseType   stageID = "warehouse_type"
	stageWarehouseAuth   stageID = "warehouse_auth"
	stageWarehouseConfig stageID = "warehouse_config"
	stageWarehouseAccess stageID = "warehouse_access"
	stageSources         stageID = "sources"
	stageIdentity        stageID = "identity"
)

type stageSpec struct {
	ID        stageID
	DependsOn []stageID
}

var stagePlan = []stageSpec{
	{ID: stagePrerequisites},
	{ID: stageWarehouseType, DependsOn: []stageID{stagePrerequisites}},
	{ID: stageWarehouseAuth, DependsOn: []stageID{stageWarehouseType}},
	{ID: stageWarehouseConfig, DependsOn: []stageID{stageWarehouseAuth}},
	{ID: stageWarehouseAccess, DependsOn: []stageID{stageWarehouseConfig}},
	{ID: stageSources, DependsOn: []stageID{stageWarehouseAccess}},
	{ID: stageIdentity, DependsOn: []stageID{stageSources}},
}

type blocker struct {
	StageID     stageID
	Status      string
	NextAction  cliresult.NextAction
	Diagnostics []cliresult.Diagnostic
}

type evaluation struct {
	completed map[stageID]bool
	blocker   *blocker
	ready     bool
}

func newEvaluation() evaluation {
	return evaluation{completed: map[stageID]bool{}}
}

func (eval *evaluation) complete(id stageID) {
	eval.completed[id] = true
}

func (eval evaluation) withBlocker(blocker blocker) evaluation {
	eval.blocker = &blocker
	return eval
}

func resultFor(envelope cliresult.Envelope, eval evaluation) Result {
	envelope.Ready = eval.ready
	envelope.Stages = buildStages(stagePlan, eval.completed, eval.blocker)
	if eval.blocker == nil {
		envelope.NextAction = doneAction()
		return Result{Envelope: envelope, ExitCode: cliresult.ExitReady}
	}

	envelope.NextAction = eval.blocker.NextAction
	envelope.Diagnostics = eval.blocker.Diagnostics
	return Result{Envelope: envelope, ExitCode: cliresult.ExitReady}
}

func buildStages(plan []stageSpec, completed map[stageID]bool, blocker *blocker) []cliresult.Stage {
	stages := make([]cliresult.Stage, 0, len(plan))
	for _, spec := range plan {
		status := statusPending
		if completed[spec.ID] && dependenciesCompleted(spec, completed) {
			status = completedStageStatus(spec.ID)
		}
		current := false
		if blocker != nil && blocker.StageID == spec.ID {
			status = blocker.Status
			current = true
		}
		stages = append(stages, cliresult.Stage{
			ID:      string(spec.ID),
			Status:  status,
			Current: current,
		})
	}
	return stages
}

func dependenciesCompleted(spec stageSpec, completed map[stageID]bool) bool {
	for _, dependency := range spec.DependsOn {
		if !completed[dependency] {
			return false
		}
	}
	return true
}

func completedStageStatus(id stageID) string {
	return statusSatisfied
}
