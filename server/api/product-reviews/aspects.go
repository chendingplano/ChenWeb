package productreviews

import "context"

// AttachAspects adds an `aspect` node to the profile root for every configured
// aspect not already present, carrying that aspect's relation-type mapping onto
// the node so retrieval need not re-read config at query time (spec: Aspect
// nodes from configured vocabulary). Aspects taken from configuration use
// origin `user_added`. Returns the number of aspect nodes inserted.
func (s Store) AttachAspects(ctx context.Context, profileID int64, cfg *Config) (int, error) {
	if cfg == nil || len(cfg.aspectOrder) == 0 {
		return 0, nil
	}
	existing, err := s.loadScanned(ctx, s.DB, profileID)
	if err != nil {
		return 0, err
	}
	present := map[string]bool{}
	for _, n := range existing {
		if n.NodeKind == KindAspect && n.AspectKey != "" {
			present[n.AspectKey] = true
		}
	}

	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	inserted := 0
	for _, key := range cfg.aspectOrder {
		if present[key] {
			continue
		}
		a := cfg.Aspects[key]
		if _, err := insertNode(ctx, tx, ProfileNode{
			ProfileID:     profileID,
			ParentNodeID:  nil, // aspects attach to the root
			NodeKind:      KindAspect,
			Label:         a.LocalizedName(""),
			LabelEN:       a.NameEN,
			Depth:         1,
			Origin:        OriginUserAdded,
			Status:        StatusAccepted,
			AspectKey:     key,
			RelationTypes: a.RelationTypes,
			MatchMode:     a.ResolvedMatchMode(),
		}); err != nil {
			return 0, err
		}
		inserted++
	}
	if inserted > 0 {
		if err := bumpVersion(ctx, tx, profileID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return inserted, nil
}
