package deployment

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// repoRoot is the ChenWeb root seen from this package directory.
const repoRoot = "../../.."

var stepCall = regexp.MustCompile(`(?m)^\s*step\s+([a-z][a-z-]*)\b`)

func scriptStepKeys(t *testing.T, files ...string) []string {
	t.Helper()
	seen := map[string]bool{}
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(repoRoot, f))
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range stepCall.FindAllStringSubmatch(string(data), -1) {
			seen[m[1]] = true
		}
	}
	keys := make([]string, 0, len(seen))
	for k := range seen {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func planKeys(kind string) []string {
	var keys []string
	for _, p := range Plan(kind) {
		if !slices.Contains(keys, p.Key) {
			keys = append(keys, p.Key)
		}
	}
	slices.Sort(keys)
	return keys
}

func TestScriptMarkersMatchPlans(t *testing.T) {
	if got, want := scriptStepKeys(t, "shell_server_build_rcp.sh", "mise.toml"), planKeys(KindBuild); !slices.Equal(got, want) {
		t.Errorf("build script markers %v, plan %v", got, want)
	}
	if got, want := scriptStepKeys(t, "shell_server_deploy.sh", "scripts/deploy-server-china.sh"), planKeys(KindDeploy); !slices.Equal(got, want) {
		t.Errorf("deploy script markers %v, plan %v", got, want)
	}
}

func TestScriptBinaryListsMatchPlans(t *testing.T) {
	mise, err := os.ReadFile(filepath.Join(repoRoot, "mise.toml"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`(?m)^ALL="([^"]+)"`).FindSubmatch(mise)
	if m == nil {
		t.Fatal("ALL= not found in mise.toml")
	}
	var built []string
	for _, pair := range strings.Fields(string(m[1])) {
		built = append(built, strings.SplitN(pair, ":", 2)[0])
	}
	if !slices.Equal(built, BuildBinaries) {
		t.Errorf("mise build order %v, BuildBinaries %v", built, BuildBinaries)
	}

	deploy, err := os.ReadFile(filepath.Join(repoRoot, "scripts/deploy-server-china.sh"))
	if err != nil {
		t.Fatal(err)
	}
	v := regexp.MustCompile(`(?m)^valid_names="([^"]+)"`).FindSubmatch(deploy)
	if v == nil {
		t.Fatal("valid_names= not found in deploy-server-china.sh")
	}
	installed := strings.Fields(string(v[1]))
	slices.Sort(installed) // the script installs *-linux in glob (sorted) order
	if !slices.Equal(installed, DeployBinaries) {
		t.Errorf("deploy names (sorted) %v, DeployBinaries %v", installed, DeployBinaries)
	}
}
