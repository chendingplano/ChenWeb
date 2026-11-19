// Package deployment backs the Deployment page (openspec deployment-page): building a
// deployment archive on the build machine, deploying it on the deployment machine, and
// listing and restarting the deployment machine's services.
//
// The build and deploy procedures stay in the shell scripts. They print one marker line
// when a step begins (`::step::<key>` or `::step::<key>::<arg>`), and deploy-runner turns
// those markers into kb.deployment_steps rows.
package deployment

const (
	KindBuild  = "build"
	KindDeploy = "deploy"

	RoleBuild  = "build"
	RoleDeploy = "deploy"
)

// PlanStep is one step the page shows before the run reaches it.
type PlanStep struct {
	Key string `json:"key"`
	Arg string `json:"arg"`
}

// BuildBinaries is the order mise build-server-linux builds the binaries in.
var BuildBinaries = []string{"server", "doc-processor", "parser-result-converter", "doc-service", "create-admin", "deploy-runner", "process-monitor", "doc-recovery"}

// DeployBinaries is the order deploy-server-china.sh installs them in (sorted *-linux names).
var DeployBinaries = []string{"create-admin", "deploy-runner", "doc-processor", "doc-recovery", "doc-service", "parser-result-converter", "process-monitor", "server"}

func binarySteps(names []string) []PlanStep {
	steps := make([]PlanStep, 0, len(names))
	for _, name := range names {
		steps = append(steps, PlanStep{Key: "binary", Arg: name})
	}
	return steps
}

// Plan returns the planned steps of a build or a deployment, in order.
func Plan(kind string) []PlanStep {
	var steps []PlanStep
	switch kind {
	case KindBuild:
		steps = append(steps, PlanStep{Key: "check"}, PlanStep{Key: "web"}, PlanStep{Key: "prompt-defaults"})
		steps = append(steps, binarySteps(BuildBinaries)...)
		steps = append(steps, PlanStep{Key: "stage"}, PlanStep{Key: "documents"}, PlanStep{Key: "pdf-parser"},
			PlanStep{Key: "release"}, PlanStep{Key: "archive"}, PlanStep{Key: "upload"}, PlanStep{Key: "release-commit"})
	case KindDeploy:
		steps = append(steps, PlanStep{Key: "check"}, PlanStep{Key: "fetch"}, PlanStep{Key: "extract"},
			PlanStep{Key: "documents"}, PlanStep{Key: "release"}, PlanStep{Key: "migrations"}, PlanStep{Key: "prompts"}, PlanStep{Key: "configs"})
		steps = append(steps, binarySteps(DeployBinaries)...)
		steps = append(steps, PlanStep{Key: "pdf-parser"})
	}
	return steps
}
