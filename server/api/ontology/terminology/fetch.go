// Fetch-side tooling for the external terminology portfolio. Downloads are an
// explicit operator bootstrap step separate from import: Fetch writes local
// artifacts, computes SHA-256, and drafts an unapproved manifest
// (license_review_status=pending_review) that terminology-import refuses until
// an operator completes the license review and approval fields. Import
// commands themselves never fetch live URLs.
package terminology

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/chendingplano/deepdoc/server/api/ontology/keywords"
)

// LicenseReviewPending marks a fetched manifest draft that has not passed the
// operator license review. It is intentionally not a value accepted by
// keywords.SourcePolicy.Validate, so a draft can never be imported.
const LicenseReviewPending = "pending_review"

// LicenseReviewDisapproved marks a fetched manifest draft that the operator
// reviewed and rejected. Like LicenseReviewPending it is not importable.
const LicenseReviewDisapproved = "disapproved"

// Approval-state sentinel errors returned by ApproveDraft so the handler can
// map them to client-visible status codes without string matching.
var (
	// ErrNotDownloaded means the source has no downloaded artifact.
	ErrNotDownloaded = errors.New("resource is not downloaded")
	// ErrNoDraftManifest means the source has no manifest.draft.json to approve.
	ErrNoDraftManifest = errors.New("resource has no draft manifest to approve")
	// ErrAlreadyApproved means the draft manifest already passed review.
	ErrAlreadyApproved = errors.New("draft manifest is already approved")
	// ErrAlreadyDisapproved means the draft manifest was already rejected.
	ErrAlreadyDisapproved = errors.New("draft manifest is already disapproved")
)

const defaultUserAgent = "ChenWeb terminology-fetch/1.0 (go)"

// ResourceID identifies one portfolio resource the page can manage.
type ResourceID string

// The five portfolio resources. Four are freely downloadable; IEC 60050-845
// (IEV) is copyright-gated and requires a licensed reviewed seed file.
const (
	ResourceQUDT     ResourceID = "qudt"
	ResourceUCUM     ResourceID = "ucum"
	ResourceSIRP     ResourceID = "sirp"
	ResourceWikidata ResourceID = "wikidata"
	ResourceIEC      ResourceID = "iec-60050-845"
)

// Resource is the static catalog entry shown on the admin page. Governance
// fields mirror the fixture manifests so a draft manifest can be written
// directly from one entry.
type Resource struct {
	ID                 ResourceID
	Name               string
	Description        string
	URL                string
	Release            string
	License            string
	LicenseURL         string
	Downloadable       bool
	PermissionRequired bool
	Notes              string
	Artifact           string
	MediaType          string
	MaxBytes           int64
	// ExpectedSizeBytes is the typical artifact size in bytes shown before
	// download; 0 means the size varies with the snapshot (for example the
	// weekly Wikidata pilot subset).
	ExpectedSizeBytes int64
	// Cadence is the upstream update cadence shown on the admin page
	// ("weekly" for Wikidata); empty means the release is pinned/fixed.
	Cadence string
	// Accept is the HTTP Accept header requested from the upstream endpoint
	// when non-empty. SIRP's /quantities endpoint content-negotiates Turtle
	// and returns a flat JSON array otherwise.
	Accept                 string
	ProviderID             string
	Source                 string
	SourceSubset           string
	Adapter                string
	AdapterVersion         string
	AuthorityRole          keywords.AuthorityRole
	AuthoritativeRelations []string
	AllowedScopes          []string
	Languages              []string
	IdentityAuthority      bool
}

var resources = []Resource{
	{
		ID: ResourceQUDT, Name: "QUDT Quantity Kinds & Units",
		Description: "QUDT 3.5.0 catalog: quantity kinds, units, and dimension vectors as one Turtle graph.",
		URL:         "https://qudt.org/download/3.5.0/qudt-all.ttl",
		Release:     "3.5.0", License: "CC-BY-4.0",
		LicenseURL:   "https://www.qudt.org/catalog/qudt-catalog.html",
		Downloadable: true, Artifact: "qudt-all.ttl", MediaType: "text/turtle", MaxBytes: 256 << 20,
		ExpectedSizeBytes: 6_858_424, // measured 2026-08-07 qudt-all.ttl
		ProviderID:        "qudt", Source: "qudt", SourceSubset: "catalog",
		Adapter: "qudt-quantity-kind", AdapterVersion: "3.5.0",
		AuthorityRole: keywords.ExactIdentityAuthority, AuthoritativeRelations: []string{"exact_equivalent"},
		AllowedScopes: []string{"quantity"}, Languages: []string{"en", "fr", "zh", "und"},
		IdentityAuthority: true,
		Notes:             "CC-BY-4.0 for units/quantity-kinds; some scale vocabularies are CC-BY-SA 3.0 US.",
	},
	{
		ID: ResourceUCUM, Name: "UCUM Essence",
		Description: "UCUM 2.2 essence XML: the case-sensitive prefixes and units used for unit-code identity.",
		URL:         "https://raw.githubusercontent.com/ucum-org/ucum/v2.2/ucum-essence.xml",
		Release:     "2.2", License: "UCUM-License-1.1",
		LicenseURL:   "https://ucum.org/license",
		Downloadable: true, Artifact: "ucum-essence.xml", MediaType: "application/xml", MaxBytes: 16 << 20,
		ExpectedSizeBytes: 2 << 20, // ucum-essence.xml is ~1.5 MB
		ProviderID:        "ucum", Source: "ucum", SourceSubset: "essence",
		Adapter: "ucum", AdapterVersion: "0.1.0",
		AuthorityRole: keywords.ContextOnly, AuthoritativeRelations: nil,
		AllowedScopes: []string{"display"}, Languages: []string{"en"},
		IdentityAuthority: false,
		Notes:             "No-charge, royalty-free UCUM License 1.1; no registration required.",
	},
	{
		ID: ResourceSIRP, Name: "BIPM SI Reference Point",
		Description: "BIPM SIRP 1.0.0 quantities index (149 quantities with persistent identifiers and expressions).",
		URL:         "https://si-digital-framework.org/quantities",
		Release:     "1.0.0", License: "CC-BY-3.0-IGO",
		LicenseURL:   "https://github.com/TheBIPM/SI_Digital_Framework/blob/main/LICENCE",
		Downloadable: true, Artifact: "quantities.ttl", MediaType: "text/turtle", MaxBytes: 16 << 20,
		Accept:            "text/turtle", // /quantities returns JSON unless Turtle is requested
		ExpectedSizeBytes: 64 << 10,      // measured 2026-08-07 quantities.ttl is ~48 KB
		ProviderID:        "bipm", Source: "bipm-sirp-quantity", SourceSubset: "quantities",
		Adapter: "bipm-sirp-quantity", AdapterVersion: "1.0.0",
		AuthorityRole: keywords.ExactIdentityAuthority, AuthoritativeRelations: []string{"exact_equivalent"},
		AllowedScopes: []string{"quantity"}, Languages: []string{"en", "fr"},
		IdentityAuthority: true,
		Notes:             "Served per-resource over the SI Digital Framework API; no bulk snapshot exists.",
	},
	{
		ID: ResourceWikidata, Name: "Wikidata (QUDT-linked entities)",
		Description:  "Revision-pinned Wikidata proposal snapshot for the QUDT-linked quantity-kind and unit entities (multilingual labels, aliases, external ids, statements).",
		URL:          "https://www.wikidata.org/w/api.php",
		Release:      "", // set at fetch time: dump-<YYYY-MM-DD>
		License:      "CC0-1.0",
		LicenseURL:   "https://www.wikidata.org/wiki/Wikidata:Licensing",
		Downloadable: true, Artifact: "snapshot.jsonl", MediaType: "application/x-ndjson", MaxBytes: 32 << 20,
		ExpectedSizeBytes: 1_163_008, // measured 2026-08-08 QUDT-linked snapshot; varies with entity revisions
		Cadence:           "weekly",  // Wikipedia/Wikidata publishes fresh dumps weekly
		ProviderID:        "wikimedia", Source: "wikidata", SourceSubset: "pilot",
		Adapter: "wikidata", AdapterVersion: "0.1.0",
		AuthorityRole: keywords.ProposalOnly, AuthoritativeRelations: nil,
		AllowedScopes: []string{"display"}, Languages: []string{"en", "zh"},
		IdentityAuthority: false,
		Notes:             "CC0 dump/API data. Full dumps are multi-GB; this fetches the entities QUDT 3.5.0 links via qudt:wikidataMatch (1,521 Q-IDs: quantity kinds and units), batched 50 per wbgetentities request.",
	},
	{
		ID: ResourceIEC, Name: "IEC 60050-845 (IEV)",
		Description: "International Electrotechnical Vocabulary, chapter 845 (lighting).",
		URL:         "https://www.electropedia.org/",
		Release:     "2020", License: "IEC copyright (licensed)",
		LicenseURL:   "https://webstore.iec.ch/en/copyright",
		Downloadable: false, PermissionRequired: true,
		Notes: "IEC content is copyright-gated. Browsing electropedia.org does not grant bulk reuse; provide a licensed, reviewed seed file instead.",
	},
}

// Resources returns the catalog sorted by ID for stable display order.
func Resources() []Resource {
	out := make([]Resource, len(resources))
	copy(out, resources)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ResourceByID returns one catalog entry.
func ResourceByID(id ResourceID) (Resource, bool) {
	for _, r := range resources {
		if r.ID == id {
			return r, true
		}
	}
	return Resource{}, false
}

// Dir resolves the fetch storage directory from TERMINOLOGY_DIR, else
// <DATA_HOME_DIR>/terminology. Empty means neither is configured.
func Dir() string {
	if v := strings.TrimSpace(os.Getenv("TERMINOLOGY_DIR")); v != "" {
		return v
	}
	if home := strings.TrimSpace(os.Getenv("DATA_HOME_DIR")); home != "" {
		return filepath.Join(home, "terminology")
	}
	return ""
}

// FetchStatus is the persisted per-source download state under
// <terminology-dir>/<source>/status.json.
type FetchStatus struct {
	Source          string    `json:"source"`
	Release         string    `json:"release"`
	Downloaded      bool      `json:"downloaded"`
	Downloading     bool      `json:"downloading,omitempty"`
	DownloadedBytes int64     `json:"downloaded_bytes,omitempty"`
	TotalBytes      int64     `json:"total_bytes,omitempty"`
	DownloadedAt    time.Time `json:"downloaded_at,omitempty"`
	SHA256          string    `json:"sha256,omitempty"`
	SizeBytes       int64     `json:"size_bytes,omitempty"`
	Artifact        string    `json:"artifact,omitempty"`
	SourceURL       string    `json:"source_url,omitempty"`
	ManifestDraft   string    `json:"manifest_draft,omitempty"`
	Error           string    `json:"error,omitempty"`
}

type fetchConfig struct {
	client      *http.Client
	urlOverride map[ResourceID]string
	wikidata    []string
	now         time.Time
}

// FetchOption customizes one download (tests use URL overrides + clients).
type FetchOption func(*fetchConfig)

// WithClient replaces the default HTTP client.
func WithClient(c *http.Client) FetchOption {
	return func(cfg *fetchConfig) { cfg.client = c }
}

// WithURLOverride points one resource at a different source URL (tests).
func WithURLOverride(id ResourceID, u string) FetchOption {
	return func(cfg *fetchConfig) { cfg.urlOverride[id] = u }
}

// WithWikidataIDs sets the Wikidata entity IDs (Q-IDs) fetched for the
// snapshot. Defaults to the QUDT-linked entity set.
func WithWikidataIDs(ids []string) FetchOption {
	return func(cfg *fetchConfig) { cfg.wikidata = ids }
}

// WithNow pins the retrieval clock (tests).
func WithNow(now time.Time) FetchOption {
	return func(cfg *fetchConfig) { cfg.now = now }
}

//go:embed wikidata_entities.txt
var wikidataEntitiesText string

var defaultWikidataEntityIDs = sync.OnceValue(func() []string {
	ids := []string{}
	for _, line := range strings.Split(wikidataEntitiesText, "\n") {
		if id := strings.TrimSpace(line); id != "" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
})

// Fetch downloads one resource into destDir/<source>/ and records status.json
// plus an unapproved manifest draft. It fails for permission-gated resources
// (IEC) and for any HTTP/checksum-unverifiable result.
func Fetch(ctx context.Context, id ResourceID, destDir string, opts ...FetchOption) (FetchStatus, error) {
	res, ok := ResourceByID(id)
	if !ok {
		return FetchStatus{}, fmt.Errorf("unknown terminology resource %q", id)
	}
	if !res.Downloadable {
		return FetchStatus{}, fmt.Errorf("resource %q requires permission and cannot be downloaded automatically", id)
	}

	cfg := fetchConfig{client: &http.Client{Timeout: 10 * time.Minute}, urlOverride: map[ResourceID]string{}, now: time.Now().UTC()}
	for _, opt := range opts {
		opt(&cfg)
	}
	client := cfg.client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Minute}
	}

	sourceDir := filepath.Join(destDir, string(id))
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		return FetchStatus{}, fmt.Errorf("create %s: %w", sourceDir, err)
	}

	sourceURL := res.URL
	if override, ok := cfg.urlOverride[id]; ok && override != "" {
		sourceURL = override
	}
	artifactPath := filepath.Join(sourceDir, res.Artifact)

	// Mark the download as in progress so the admin page can render progress
	// while it streams; the final write below overwrites this state.
	rel := res.Release
	_ = writeStatusFile(sourceDir, FetchStatus{Source: string(id), Release: rel, Downloading: true})

	lastProgress := time.Time{}
	onProgress := func(done, total int64) {
		now := time.Now().UTC()
		if now.Sub(lastProgress) < 500*time.Millisecond {
			return
		}
		lastProgress = now
		_ = writeStatusFile(sourceDir, FetchStatus{
			Source: string(id), Release: rel, Downloading: true,
			DownloadedBytes: done, TotalBytes: total,
		})
	}

	var (
		sha  string
		size int64
		err  error
	)
	switch id {
	case ResourceWikidata:
		ids := cfg.wikidata
		if len(ids) == 0 {
			ids = defaultWikidataEntityIDs()
		}
		sha, size, sourceURL, err = fetchWikidataSnapshot(ctx, client, sourceURL, artifactPath, ids, res.MaxBytes, cfg.now, onProgress)
		if rel == "" {
			rel = "dump-" + cfg.now.Format("2006-01-02")
		}
	default:
		sha, size, err = downloadToFile(ctx, client, sourceURL, artifactPath, res.MaxBytes, res.Accept, onProgress)
	}
	if err != nil {
		st := FetchStatus{Source: string(id), Release: rel, Downloaded: false, Error: err.Error()}
		_ = writeStatusFile(sourceDir, st)
		return st, err
	}

	st := FetchStatus{
		Source: string(id), Release: rel, Downloaded: true, DownloadedAt: cfg.now,
		SHA256: sha, SizeBytes: size, Artifact: res.Artifact,
		SourceURL: sourceURL, ManifestDraft: "manifest.draft.json",
	}
	if err := writeDraftManifest(sourceDir, res, st); err != nil {
		return st, fmt.Errorf("write draft manifest: %w", err)
	}
	if err := writeStatusFile(sourceDir, st); err != nil {
		return st, err
	}
	return st, nil
}

// DraftReviewStatus returns the license_review_status recorded in the
// source's draft manifest: LicenseReviewPending while the fetch is fresh,
// "approved" if the operator edited the draft in place, or "" when no draft
// exists (never fetched, or already approved and moved to a real manifest).
func DraftReviewStatus(destDir string, id ResourceID) (string, error) {
	b, err := os.ReadFile(filepath.Join(destDir, string(id), "manifest.draft.json"))
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read %s draft manifest: %w", id, err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return "", fmt.Errorf("decode %s draft manifest: %w", id, err)
	}
	return strings.TrimSpace(m.Policy.LicenseReviewStatus), nil
}

// ApproveDraft completes the operator license review of one downloaded
// source's draft manifest in place: it sets license_review_status to approved
// and records approved_by/approved_at plus the review comments and reviewer.
// It fails closed when the source is not downloaded, has no draft, or has
// already been decided. The approved manifest is still a local file; importing
// it is a separate step (Runner.Import).
func ApproveDraft(destDir string, id ResourceID, approvedBy, comments string, at time.Time) (FetchStatus, error) {
	if strings.TrimSpace(approvedBy) == "" {
		return FetchStatus{}, errors.New("approved_by is required")
	}
	st, manifestPath, m, err := loadReviewableDraft(destDir, id)
	if err != nil {
		return st, err
	}
	if decided(m) {
		return st, fmt.Errorf("%w: %s", alreadyDecidedErr(m), id)
	}
	m.Policy.LicenseReviewStatus = keywords.LicenseReviewApproved
	m.Policy.ApprovedBy = strings.TrimSpace(approvedBy)
	m.Policy.ApprovedAt = &at
	m.Policy.ReviewComments = strings.TrimSpace(comments)
	m.Policy.ReviewedBy = strings.TrimSpace(approvedBy)
	m.Policy.ReviewedAt = &at
	if err := writeManifest(manifestPath, m); err != nil {
		return st, err
	}
	return st, nil
}

// DisapproveDraft records an operator rejection: license_review_status becomes
// LicenseReviewDisapproved and the review comments and reviewer are saved. It
// fails closed like ApproveDraft, and never imports anything.
func DisapproveDraft(destDir string, id ResourceID, reviewer, comments string, at time.Time) (FetchStatus, error) {
	if strings.TrimSpace(reviewer) == "" {
		return FetchStatus{}, errors.New("reviewer is required")
	}
	st, manifestPath, m, err := loadReviewableDraft(destDir, id)
	if err != nil {
		return st, err
	}
	if decided(m) {
		return st, fmt.Errorf("%w: %s", alreadyDecidedErr(m), id)
	}
	m.Policy.LicenseReviewStatus = LicenseReviewDisapproved
	m.Policy.ReviewComments = strings.TrimSpace(comments)
	m.Policy.ReviewedBy = strings.TrimSpace(reviewer)
	m.Policy.ReviewedAt = &at
	if err := writeManifest(manifestPath, m); err != nil {
		return st, err
	}
	return st, nil
}

// loadReviewableDraft returns the persisted status, draft manifest path, and
// decoded manifest for a downloaded source with a draft to review.
func loadReviewableDraft(destDir string, id ResourceID) (FetchStatus, string, Manifest, error) {
	st, err := ReadStatus(destDir, id)
	if err != nil {
		return st, "", Manifest{}, err
	}
	if !st.Downloaded {
		return st, "", Manifest{}, fmt.Errorf("%w: %s", ErrNotDownloaded, id)
	}
	manifestPath := filepath.Join(destDir, string(id), "manifest.draft.json")
	b, err := os.ReadFile(manifestPath)
	if errors.Is(err, os.ErrNotExist) {
		return st, "", Manifest{}, fmt.Errorf("%w: %s", ErrNoDraftManifest, id)
	}
	if err != nil {
		return st, "", Manifest{}, fmt.Errorf("read %s draft manifest: %w", id, err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return st, "", Manifest{}, fmt.Errorf("decode %s draft manifest: %w", id, err)
	}
	return st, manifestPath, m, nil
}

func decided(m Manifest) bool {
	return m.Policy.LicenseReviewStatus == keywords.LicenseReviewApproved ||
		m.Policy.LicenseReviewStatus == LicenseReviewDisapproved
}

func alreadyDecidedErr(m Manifest) error {
	if m.Policy.LicenseReviewStatus == LicenseReviewDisapproved {
		return ErrAlreadyDisapproved
	}
	return ErrAlreadyApproved
}

func writeManifest(manifestPath string, m Manifest) error {
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicWrite(manifestPath, b); err != nil {
		return fmt.Errorf("write draft manifest: %w", err)
	}
	return nil
}

// DraftReview is the operator review state recorded in a source's draft
// manifest: disposition (pending_review | approved | disapproved), comments,
// and reviewer identity/time when a decision was made.
type DraftReview struct {
	Status     string     `json:"status"`
	Comments   string     `json:"comments,omitempty"`
	ReviewedBy string     `json:"reviewed_by,omitempty"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
}

// ReadDraftReview returns the full review record for a source's draft
// manifest. A missing draft yields an empty review (status "").
func ReadDraftReview(destDir string, id ResourceID) (DraftReview, error) {
	b, err := os.ReadFile(filepath.Join(destDir, string(id), "manifest.draft.json"))
	if errors.Is(err, os.ErrNotExist) {
		return DraftReview{}, nil
	}
	if err != nil {
		return DraftReview{}, fmt.Errorf("read %s draft manifest: %w", id, err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return DraftReview{}, fmt.Errorf("decode %s draft manifest: %w", id, err)
	}
	dr := DraftReview{
		Status:     strings.TrimSpace(m.Policy.LicenseReviewStatus),
		Comments:   strings.TrimSpace(m.Policy.ReviewComments),
		ReviewedBy: strings.TrimSpace(m.Policy.ReviewedBy),
		ReviewedAt: m.Policy.ReviewedAt,
	}
	return dr, nil
}

// ReadStatus loads one resource's persisted status; a never-fetched resource
// returns Downloaded=false.
func ReadStatus(destDir string, id ResourceID) (FetchStatus, error) {
	b, err := os.ReadFile(filepath.Join(destDir, string(id), "status.json"))
	if errors.Is(err, os.ErrNotExist) {
		return FetchStatus{Source: string(id)}, nil
	}
	if err != nil {
		return FetchStatus{}, err
	}
	var st FetchStatus
	if err := json.Unmarshal(b, &st); err != nil {
		return FetchStatus{}, fmt.Errorf("decode %s status: %w", id, err)
	}
	return st, nil
}

func writeStatusFile(sourceDir string, st FetchStatus) error {
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(sourceDir, "status.json"), b)
}

// writeDraftManifest emits an unapproved manifest (pending_review, no
// approved_by/approved_at) so terminology-import fails closed until the
// operator completes the license review.
func writeDraftManifest(sourceDir string, res Resource, st FetchStatus) error {
	m := Manifest{
		Adapter: res.Adapter,
		Policy: ManifestPolicy{
			ProviderID: res.ProviderID, Source: res.Source, SourceSubset: res.SourceSubset,
			Release: st.Release, RetrievedAt: st.DownloadedAt, ContentChecksum: st.SHA256,
			License: res.License, LicenseReviewStatus: LicenseReviewPending,
			AuthorityRole: res.AuthorityRole, AuthoritativeRelations: res.AuthoritativeRelations,
			AllowedScopes: res.AllowedScopes, Languages: res.Languages,
			AdapterVersion: res.AdapterVersion, ProvenanceLocator: st.SourceURL,
			IdentityAuthority: res.IdentityAuthority,
			Notes:             "downloaded by terminology-fetch; awaiting operator license review and approval",
		},
		Artifacts: []ManifestArtifact{{
			ID: res.Artifact, Path: res.Artifact, SHA256: st.SHA256,
			MediaType: res.MediaType, ProvenanceLocator: st.SourceURL,
		}},
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(filepath.Join(sourceDir, "manifest.draft.json"), b)
}

func atomicWrite(path string, b []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".write-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func downloadToFile(ctx context.Context, client *http.Client, sourceURL, destPath string, maxBytes int64, accept string, onProgress func(done, total int64)) (string, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return "", 0, err
	}
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", 0, fmt.Errorf("GET %s: %s", sourceURL, resp.Status)
	}
	total := resp.ContentLength
	if total < 0 {
		total = 0
	}
	return writeStreamed(resp.Body, destPath, maxBytes, func(written int64) {
		if onProgress != nil {
			onProgress(written, total)
		}
	})
}

// progressWriter reports bytes written through it without storing them, so
// io.Copy can stream progress for one download.
type progressWriter struct {
	cb func(int64)
}

func (w *progressWriter) Write(b []byte) (int, error) {
	if w.cb != nil {
		w.cb(int64(len(b)))
	}
	return len(b), nil
}

func writeStreamed(r io.Reader, destPath string, maxBytes int64, onProgress func(written int64)) (string, int64, error) {
	tmp, err := os.CreateTemp(filepath.Dir(destPath), ".fetch-*")
	if err != nil {
		return "", 0, err
	}
	defer os.Remove(tmp.Name())
	h := sha256.New()
	writers := []io.Writer{tmp, h}
	if onProgress != nil {
		writers = append(writers, &progressWriter{cb: onProgress})
	}
	written, err := io.Copy(io.MultiWriter(writers...), io.LimitReader(r, maxBytes+1))
	if err != nil {
		tmp.Close()
		return "", 0, err
	}
	if written > maxBytes {
		tmp.Close()
		return "", 0, fmt.Errorf("response exceeds %d bytes", maxBytes)
	}
	if err := tmp.Close(); err != nil {
		return "", 0, err
	}
	if err := os.Rename(tmp.Name(), destPath); err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), written, nil
}

// wikidataBatchSize is the wbgetentities per-request entity limit for
// non-bot clients. Batch delay and 429 backoff keep the weekly refresh within
// Wikimedia's anonymous rate limits.
const (
	wikidataBatchSize   = 50
	wikidataBatchDelay  = time.Second
	wikidataMaxAttempts = 8
	wikidataMaxBackoff  = 60 * time.Second
)

// fetchWikidataSnapshot calls wbgetentities for the requested entity IDs in
// batches and writes one normalized WikidataLine JSON object per resolved
// entity as JSONL. Missing/redirected entities are skipped. It returns the
// effective source URL of the first batch.
func fetchWikidataSnapshot(ctx context.Context, client *http.Client, baseURL, destPath string, ids []string, maxBytes int64, now time.Time, onProgress func(done, total int64)) (string, int64, string, error) {
	if len(ids) == 0 {
		return "", 0, "", errors.New("wikidata entity list is empty")
	}
	// Sort a copy so batching and snapshot order are deterministic regardless
	// of how the caller ordered the IDs.
	ids = append([]string(nil), ids...)
	sort.Strings(ids)
	effectiveURL := ""
	entities := map[string]wikidataRawEntity{}
	downloaded := int64(0)
	for start := 0; start < len(ids); start += wikidataBatchSize {
		end := start + wikidataBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		batch := ids[start:end]
		u, err := url.Parse(baseURL)
		if err != nil {
			return "", 0, "", err
		}
		q := u.Query()
		q.Set("action", "wbgetentities")
		q.Set("ids", strings.Join(batch, "|"))
		q.Set("props", "info|labels|aliases|claims")
		q.Set("languages", "en|zh|fr")
		q.Set("format", "json")
		u.RawQuery = q.Encode()
		batchURL := u.String()
		if effectiveURL == "" {
			effectiveURL = batchURL
		}

		var body []byte
		for attempt := 0; attempt < wikidataMaxAttempts; attempt++ {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, batchURL, nil)
			if err != nil {
				return "", 0, "", err
			}
			req.Header.Set("User-Agent", defaultUserAgent)
			resp, err := client.Do(req)
			if err != nil {
				return "", 0, "", err
			}
			if resp.StatusCode == http.StatusTooManyRequests && attempt < wikidataMaxAttempts-1 {
				wait := time.Duration(1<<attempt) * time.Second
				if ra := resp.Header.Get("Retry-After"); ra != "" {
					if seconds, err := strconv.Atoi(ra); err == nil && seconds > 0 {
						wait = time.Duration(seconds) * time.Second
					}
				}
				if wait > wikidataMaxBackoff {
					wait = wikidataMaxBackoff
				}
				resp.Body.Close()
				select {
				case <-time.After(wait):
				case <-ctx.Done():
					return "", 0, "", ctx.Err()
				}
				continue
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return "", 0, "", fmt.Errorf("GET %s: %s", batchURL, resp.Status)
			}
			body, err = io.ReadAll(io.LimitReader(resp.Body, maxBytes+1))
			resp.Body.Close()
			if err != nil {
				return "", 0, "", err
			}
			break
		}
		if int64(len(body)) > maxBytes {
			return "", 0, "", fmt.Errorf("response exceeds %d bytes", maxBytes)
		}
		downloaded += int64(len(body))
		if onProgress != nil {
			// Total is unknown ahead of the stream; report cumulative bytes.
			onProgress(downloaded, 0)
		}
		if end < len(ids) {
			select {
			case <-time.After(wikidataBatchDelay):
			case <-ctx.Done():
				return "", 0, "", ctx.Err()
			}
		}

		var payload struct {
			Entities map[string]wikidataRawEntity `json:"entities"`
			Error    *struct {
				Info string `json:"info"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return "", 0, "", fmt.Errorf("decode wbgetentities response: %w", err)
		}
		if payload.Error != nil {
			return "", 0, "", fmt.Errorf("wikidata API error: %s", payload.Error.Info)
		}
		for id, entity := range payload.Entities {
			if entity.Missing != "" || entity.LastRevID <= 0 {
				continue // unknown or deleted entity: no revision to pin
			}
			entities[id] = entity
		}
	}
	if len(entities) == 0 {
		return "", 0, "", errors.New("wikidata API returned no entities")
	}

	var buf bytes.Buffer
	h := sha256.New()
	mw := io.MultiWriter(&buf, h)
	sorted := make([]string, 0, len(entities))
	for id := range entities {
		sorted = append(sorted, id)
	}
	sort.Strings(sorted)
	for _, id := range sorted {
		line, err := normalizeWikidataEntity(entities[id], now)
		if err != nil {
			return "", 0, "", err
		}
		raw, err := json.Marshal(line)
		if err != nil {
			return "", 0, "", err
		}
		raw = append(raw, '\n')
		if _, err := mw.Write(raw); err != nil {
			return "", 0, "", err
		}
	}
	if int64(buf.Len()) > maxBytes {
		return "", 0, "", fmt.Errorf("snapshot exceeds %d bytes", maxBytes)
	}
	if err := atomicWrite(destPath, buf.Bytes()); err != nil {
		return "", 0, "", err
	}
	if onProgress != nil {
		onProgress(int64(buf.Len()), int64(buf.Len()))
	}
	return hex.EncodeToString(h.Sum(nil)), int64(buf.Len()), effectiveURL, nil
}

// wikidataRawEntity is one entity as returned by the wbgetentities API.
type wikidataRawEntity struct {
	ID        string `json:"id"`
	LastRevID int64  `json:"lastrevid"`
	Missing   string `json:"missing"`
	Labels    map[string]struct {
		Language string `json:"language"`
		Value    string `json:"value"`
	} `json:"labels"`
	Aliases map[string][]struct {
		Language string `json:"language"`
		Value    string `json:"value"`
	} `json:"aliases"`
	Claims map[string][]wikidataRawClaim `json:"claims"`
}

type wikidataRawClaim struct {
	Mainsnak wikidataRawSnak `json:"mainsnak"`
}

type wikidataRawSnak struct {
	Snaktype  string `json:"snaktype"`
	Datatype  string `json:"datatype"`
	Datavalue *struct {
		Value json.RawMessage `json:"value"`
	} `json:"datavalue"`
}

// normalizeWikidataEntity projects one wbgetentities entity into the
// revision-pinned WikidataLine schema the adapter consumes: labels/aliases
// are flattened to language->text, external-id/url claims become
// external_ids, and P1889/P279 item claims become different_from/broader
// proposal statements.
func normalizeWikidataEntity(raw wikidataRawEntity, retrievedAt time.Time) (WikidataLine, error) {
	line := WikidataLine{
		ID: raw.ID, Revision: raw.LastRevID, RetrievedAt: retrievedAt,
		Labels: map[string]string{}, Aliases: map[string][]string{},
	}
	if strings.TrimSpace(line.ID) == "" {
		return line, errors.New("wikidata entity is missing its id")
	}
	for language, label := range raw.Labels {
		line.Labels[language] = label.Value
	}
	for language, aliases := range raw.Aliases {
		values := make([]string, 0, len(aliases))
		for _, alias := range aliases {
			values = append(values, alias.Value)
		}
		line.Aliases[language] = values
	}
	for property, claims := range raw.Claims {
		for _, claim := range claims {
			snak := claim.Mainsnak
			if snak.Snaktype != "value" || snak.Datavalue == nil {
				continue
			}
			switch snak.Datatype {
			case "external-id", "url":
				var value string
				if err := json.Unmarshal(snak.Datavalue.Value, &value); err != nil {
					continue
				}
				line.ExternalIDs = append(line.ExternalIDs, ExternalIDClaim{Property: property, Value: value})
			case "wikibase-item":
				var item struct {
					ID string `json:"id"`
				}
				if err := json.Unmarshal(snak.Datavalue.Value, &item); err != nil || item.ID == "" {
					continue
				}
				if statementType := wikidataStatementTypeForProperty(property); statementType != "" {
					line.Statements = append(line.Statements, WikidataStatement{Type: statementType, Object: item.ID})
				}
			}
		}
	}
	// Claim maps iterate in random order; sort so identical API responses
	// always produce byte-identical artifacts with a stable checksum.
	sort.Slice(line.ExternalIDs, func(i, j int) bool {
		if line.ExternalIDs[i].Property != line.ExternalIDs[j].Property {
			return line.ExternalIDs[i].Property < line.ExternalIDs[j].Property
		}
		return line.ExternalIDs[i].Value < line.ExternalIDs[j].Value
	})
	sort.Slice(line.Statements, func(i, j int) bool {
		if line.Statements[i].Type != line.Statements[j].Type {
			return line.Statements[i].Type < line.Statements[j].Type
		}
		return line.Statements[i].Object < line.Statements[j].Object
	})
	return line, nil
}

// wikidataStatementTypeForProperty maps Wikidata item-relation properties to
// the normalized proposal statement types the adapter understands. P279
// subclass-of names the broader concept on the entity; P1889 different-from
// is negative-evidence proposal. Both remain proposal-only, never authority.
func wikidataStatementTypeForProperty(property string) string {
	switch property {
	case "P1889": // different from
		return "different_from"
	case "P279": // subclass of; the object is the broader concept
		return "broader"
	}
	return ""
}
