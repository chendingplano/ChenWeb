package terminology

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
)

// Manifest pins one adapter, one immutable source policy, and every local
// artifact required to reproduce the imported snapshot.
type Manifest struct {
	Adapter   string             `json:"adapter"`
	Policy    ManifestPolicy     `json:"policy"`
	Artifacts []ManifestArtifact `json:"artifacts"`
}

type ManifestPolicy struct {
	ProviderID             string                 `json:"provider_id"`
	Source                 string                 `json:"source"`
	SourceSubset           string                 `json:"source_subset"`
	Release                string                 `json:"release"`
	RetrievedAt            time.Time              `json:"retrieved_at"`
	ContentChecksum        string                 `json:"content_checksum"`
	License                string                 `json:"license"`
	LicenseReviewStatus    string                 `json:"license_review_status"`
	AuthorityRole          keywords.AuthorityRole `json:"authority_role"`
	AuthoritativeRelations []string               `json:"authoritative_relations"`
	AllowedScopes          []string               `json:"allowed_scopes"`
	Languages              []string               `json:"languages"`
	AdapterVersion         string                 `json:"adapter_version"`
	ProvenanceLocator      string                 `json:"provenance_locator"`
	ApprovedBy             string                 `json:"approved_by"`
	ApprovedAt             *time.Time             `json:"approved_at"`
	IdentityAuthority      bool                   `json:"identity_authority"`
	Notes                  string                 `json:"notes,omitempty"`
}

type ManifestArtifact struct {
	ID                string          `json:"id"`
	Path              string          `json:"path"`
	SHA256            string          `json:"sha256"`
	MediaType         string          `json:"media_type"`
	ProvenanceLocator string          `json:"provenance_locator"`
	Metadata          json.RawMessage `json:"metadata,omitempty"`
}

type VerifiedArtifact struct {
	ManifestArtifact
	ResolvedPath string `json:"-"`
	Content      []byte `json:"-"`
}

func (p ManifestPolicy) SourcePolicy() keywords.SourcePolicy {
	return keywords.SourcePolicy{
		ProviderID: p.ProviderID, Source: p.Source, SourceSubset: p.SourceSubset,
		Release: p.Release, RetrievedAt: p.RetrievedAt, ContentChecksum: p.ContentChecksum,
		License: p.License, LicenseReviewStatus: p.LicenseReviewStatus,
		AuthorityRole: p.AuthorityRole, AuthoritativeRelations: p.AuthoritativeRelations,
		AllowedScopes: p.AllowedScopes, Languages: p.Languages, AdapterVersion: p.AdapterVersion,
		ProvenanceLocator: p.ProvenanceLocator, ApprovedBy: p.ApprovedBy,
		ApprovedAt: p.ApprovedAt, IdentityAuthority: p.IdentityAuthority, Notes: p.Notes,
	}
}

func ParseAndVerifyManifest(path string) (Manifest, []VerifiedArtifact, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("read manifest: %w", err)
	}
	if err := rejectDuplicateJSONKeys(b); err != nil {
		return Manifest{}, nil, fmt.Errorf("manifest JSON: %w", err)
	}
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, nil, fmt.Errorf("decode manifest: %w", err)
	}
	if err := requireJSONEOF(dec); err != nil {
		return Manifest{}, nil, err
	}
	if strings.TrimSpace(m.Adapter) == "" {
		return Manifest{}, nil, errors.New("manifest adapter is required")
	}
	if strings.TrimSpace(m.Policy.Release) == "" {
		return Manifest{}, nil, errors.New("manifest policy release/revision is required")
	}
	if err := m.Policy.SourcePolicy().Validate(); err != nil {
		return Manifest{}, nil, fmt.Errorf("manifest policy: %w", err)
	}
	if len(m.Artifacts) == 0 {
		return Manifest{}, nil, errors.New("manifest requires at least one artifact")
	}

	root, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("resolve manifest directory: %w", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return Manifest{}, nil, fmt.Errorf("absolute manifest directory: %w", err)
	}
	seenID, seenPath := map[string]bool{}, map[string]bool{}
	verified := make([]VerifiedArtifact, 0, len(m.Artifacts))
	for _, a := range m.Artifacts {
		if strings.TrimSpace(a.ID) == "" || strings.TrimSpace(a.MediaType) == "" || strings.TrimSpace(a.ProvenanceLocator) == "" {
			return Manifest{}, nil, errors.New("artifact id, media_type, and provenance_locator are required")
		}
		if seenID[a.ID] {
			return Manifest{}, nil, fmt.Errorf("duplicate artifact id %q", a.ID)
		}
		seenID[a.ID] = true
		if filepath.IsAbs(a.Path) || a.Path == "" {
			return Manifest{}, nil, fmt.Errorf("artifact %q path must be relative", a.ID)
		}
		joined := filepath.Join(root, filepath.Clean(a.Path))
		resolved, err := filepath.EvalSymlinks(joined)
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("resolve artifact %q: %w", a.ID, err)
		}
		resolved, err = filepath.Abs(resolved)
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("absolute artifact %q: %w", a.ID, err)
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return Manifest{}, nil, fmt.Errorf("artifact %q escapes manifest directory", a.ID)
		}
		if seenPath[resolved] {
			return Manifest{}, nil, fmt.Errorf("duplicate artifact path %q", a.Path)
		}
		seenPath[resolved] = true
		if len(a.SHA256) != 64 || strings.ToLower(a.SHA256) != a.SHA256 {
			return Manifest{}, nil, fmt.Errorf("artifact %q sha256 must be 64 lowercase hex characters", a.ID)
		}
		want, err := hex.DecodeString(a.SHA256)
		if err != nil || len(want) != sha256.Size {
			return Manifest{}, nil, fmt.Errorf("artifact %q sha256 is invalid", a.ID)
		}
		content, err := os.ReadFile(resolved)
		if err != nil {
			return Manifest{}, nil, fmt.Errorf("read artifact %q: %w", a.ID, err)
		}
		got := sha256.Sum256(content)
		if !bytes.Equal(want, got[:]) {
			return Manifest{}, nil, fmt.Errorf("artifact %q checksum mismatch", a.ID)
		}
		if len(a.Metadata) != 0 {
			var v any
			if err := json.Unmarshal(a.Metadata, &v); err != nil {
				return Manifest{}, nil, fmt.Errorf("artifact %q metadata: %w", a.ID, err)
			}
			a.Metadata, _ = json.Marshal(v)
		}
		verified = append(verified, VerifiedArtifact{ManifestArtifact: a, ResolvedPath: resolved, Content: content})
	}
	sort.Slice(verified, func(i, j int) bool { return verified[i].ID < verified[j].ID })
	return m, verified, nil
}

func CanonicalManifestJSON(m Manifest) ([]byte, error) {
	m.Policy.AuthoritativeRelations = canonicalStrings(m.Policy.AuthoritativeRelations)
	m.Policy.AllowedScopes = canonicalStrings(m.Policy.AllowedScopes)
	m.Policy.Languages = canonicalStrings(m.Policy.Languages)
	sort.Slice(m.Artifacts, func(i, j int) bool { return m.Artifacts[i].ID < m.Artifacts[j].ID })
	return json.Marshal(m)
}

func canonicalStrings(in []string) []string {
	out := append([]string(nil), in...)
	for i := range out {
		out[i] = strings.TrimSpace(out[i])
	}
	sort.Strings(out)
	n := 0
	for _, s := range out {
		if s != "" && (n == 0 || out[n-1] != s) {
			out[n] = s
			n++
		}
	}
	return out[:n]
}

func requireJSONEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("manifest contains multiple JSON values")
		}
		return fmt.Errorf("manifest trailing data: %w", err)
	}
	return nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	var walk func() error
	walk = func() error {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		delim, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for dec.More() {
				keyTok, err := dec.Token()
				if err != nil {
					return err
				}
				key := keyTok.(string)
				if seen[key] {
					return fmt.Errorf("duplicate key %q", key)
				}
				seen[key] = true
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = dec.Token()
			return err
		case '[':
			for dec.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = dec.Token()
			return err
		default:
			return nil
		}
	}
	return walk()
}
