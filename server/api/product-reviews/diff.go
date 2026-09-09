package productreviews

import "sort"

// ArtifactRef identifies a result across runs.
type ArtifactRef struct {
	ArtifactType string `json:"artifact_type"`
	ArtifactID   string `json:"artifact_id"`
	Tier         string `json:"tier"`
}

// TierChange is an artifact present in both runs whose tier moved.
type TierChange struct {
	ArtifactType string `json:"artifact_type"`
	ArtifactID   string `json:"artifact_id"`
	From         string `json:"from"`
	To           string `json:"to"`
}

// RunDiff compares a run against the previous run of the same request.
type RunDiff struct {
	Added                  []ArtifactRef `json:"added"`
	Removed                []ArtifactRef `json:"removed"`
	Retiered               []TierChange  `json:"retiered"`
	ProfileVersionChanged  bool          `json:"profile_version_changed"`
	PreviousProfileVersion int           `json:"previous_profile_version"`
	CurrentProfileVersion  int           `json:"current_profile_version"`
	PreviousRunNumber      int           `json:"previous_run_number"`
}

// DiffResults compares two result sets (previous → current). A stable-corpus,
// same-profile re-run yields empty Added / Removed / Retiered.
func DiffResults(prev, cur []Result, prevVer, curVer, prevRunNumber int) RunDiff {
	prevByKey := indexResults(prev)
	curByKey := indexResults(cur)

	d := RunDiff{
		ProfileVersionChanged:  prevVer != curVer,
		PreviousProfileVersion: prevVer,
		CurrentProfileVersion:  curVer,
		PreviousRunNumber:      prevRunNumber,
	}
	for key, r := range curByKey {
		if _, ok := prevByKey[key]; !ok {
			d.Added = append(d.Added, ArtifactRef{r.ArtifactType, r.ArtifactID, r.Tier})
		}
	}
	for key, r := range prevByKey {
		cr, ok := curByKey[key]
		if !ok {
			d.Removed = append(d.Removed, ArtifactRef{r.ArtifactType, r.ArtifactID, r.Tier})
			continue
		}
		if cr.Tier != r.Tier {
			d.Retiered = append(d.Retiered, TierChange{r.ArtifactType, r.ArtifactID, r.Tier, cr.Tier})
		}
	}

	sort.Slice(d.Added, func(i, j int) bool { return refLess(d.Added[i], d.Added[j]) })
	sort.Slice(d.Removed, func(i, j int) bool { return refLess(d.Removed[i], d.Removed[j]) })
	sort.Slice(d.Retiered, func(i, j int) bool {
		if d.Retiered[i].ArtifactType != d.Retiered[j].ArtifactType {
			return d.Retiered[i].ArtifactType < d.Retiered[j].ArtifactType
		}
		return d.Retiered[i].ArtifactID < d.Retiered[j].ArtifactID
	})
	return d
}

func indexResults(rs []Result) map[string]Result {
	out := make(map[string]Result, len(rs))
	for _, r := range rs {
		out[r.ArtifactType+"\x1f"+r.ArtifactID] = r
	}
	return out
}

func refLess(a, b ArtifactRef) bool {
	if a.ArtifactType != b.ArtifactType {
		return a.ArtifactType < b.ArtifactType
	}
	return a.ArtifactID < b.ArtifactID
}
