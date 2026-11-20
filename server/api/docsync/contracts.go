package docsync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/lib/pq"
)

const VocabularyProcessor = "vocabulary"
const bundleContractVersion = 1

// PortableRow carries the source primary key only for constructing local references.
// ReferenceKeys identify the logical rows behind numeric fields across re-runs.
type PortableRow struct {
	JSONNulls     []string                   `json:"json_nulls,omitempty"`
	SourceID      int64                      `json:"source_id,omitempty"`
	Fields        map[string]json.RawMessage `json:"fields"`
	ReferenceKeys map[string]json.RawMessage `json:"reference_keys,omitempty"`
}

type Bundle struct {
	Rows         map[string][]PortableRow `json:"rows"`
	ObjectOwners map[int64]string         `json:"object_owners,omitempty"`
}

type BundleResult struct {
	Rows      int
	Conflicts []string
}

type rowReference struct {
	column   string
	table    string
	deferred bool
}

type tableContract struct {
	name, id, scope, fields, key, jsonFields string
	refs                                     []rowReference
	shared                                   bool
}

type contractStep struct {
	table string
	where string
}

type processorContract struct {
	dependencies []string
	steps        []contractStep
}

func HasContract(processor string) bool {
	_, ok := processorContracts[processor]
	return ok
}

func ProcessorDependencies(processor string) []string {
	return slices.Clone(processorContracts[processor].dependencies)
}

func AllContracts() []string {
	return slices.Clone(processorContractOrder)
}

// Digest excludes transport IDs and hashes the sorted portable rows of each table.
func (b Bundle) Digest() (string, error) {
	tables := make(map[string][]string, len(b.Rows))
	for name, rows := range b.Rows {
		tables[name] = make([]string, 0, len(rows))
		for _, row := range rows {
			fields := cloneFields(row.Fields)
			for _, name := range strings.Fields(volatileBundleFields[name]) {
				delete(fields, name)
			}
			for col, key := range row.ReferenceKeys {
				fields[col] = key
			}
			jsonNulls := slices.Clone(row.JSONNulls)
			slices.Sort(jsonNulls)
			encoded, err := json.Marshal(struct {
				Fields    map[string]json.RawMessage `json:"fields"`
				JSONNulls []string                   `json:"json_nulls,omitempty"`
			}{fields, jsonNulls})
			if err != nil {
				return "", err
			}
			tables[name] = append(tables[name], string(encoded))
		}
		slices.Sort(tables[name])
	}
	raw, err := json.Marshal(struct {
		Version      int                 `json:"version"`
		Tables       map[string][]string `json:"tables"`
		ObjectOwners map[int64]string    `json:"object_owners,omitempty"`
	}{bundleContractVersion, tables, b.ObjectOwners})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

var volatileBundleFields = map[string]string{
	"doc_facets":       "ks_store_id",
	"doc_facet_values": "decision_attempt_id invocation_id",
	"provisions":       "extract_id",
}

func cloneFields(input map[string]json.RawMessage) map[string]json.RawMessage {
	result := make(map[string]json.RawMessage, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func sqlFields(fields string) string {
	columns := strings.Fields(fields)
	for i := range columns {
		columns[i] = pq.QuoteIdentifier(columns[i])
	}
	return strings.Join(columns, ",")
}

func rowJSON(fields string) string {
	columns := strings.Fields(fields)
	args := make([]string, 0, 2*len(columns))
	for _, col := range columns {
		args = append(args, pq.QuoteLiteral(col), "t."+pq.QuoteIdentifier(col))
	}
	// PostgreSQL accepts at most 100 arguments per function invocation.
	const maxArgs = 100
	if len(args) > maxArgs {
		return rowJSON(strings.Join(columns[:maxArgs/2], " ")) +
			" || " + rowJSON(strings.Join(columns[maxArgs/2:], " "))
	}
	return "jsonb_build_object(" + strings.Join(args, ",") + ")"
}

func sourceIDExpr(t tableContract) string {
	if t.id == "" {
		return "0::bigint"
	}
	return "t." + pq.QuoteIdentifier(t.id)
}

func matchesScope(t tableContract, predicate string) string {
	if predicate != "" {
		return predicate
	}
	if t.scope != "" {
		return "t." + pq.QuoteIdentifier(t.scope) + " = $1"
	}
	return ""
}

func queryContractRows(ctx context.Context, tx *sql.Tx, t tableContract, predicate string, args ...any) ([]PortableRow, error) {
	nulls := "ARRAY[]::text[]"
	if t.jsonFields != "" {
		var checks []string
		for _, col := range strings.Fields(t.jsonFields) {
			checks = append(checks, "CASE WHEN t."+pq.QuoteIdentifier(col)+"='null'::jsonb THEN "+pq.QuoteLiteral(col)+" END")
		}
		nulls = "array_remove(ARRAY[" + strings.Join(checks, ",") + "],NULL)"
	}
	query := "SELECT " + sourceIDExpr(t) + "," + rowJSON(t.fields) + "," + nulls + " FROM kb." + t.name + " t"
	if predicate != "" {
		query += " WHERE " + predicate
	}
	query += " ORDER BY 1, 2"
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("export %s: %w", t.name, err)
	}
	defer rows.Close()
	result := make([]PortableRow, 0)
	for rows.Next() {
		var row PortableRow
		var raw []byte
		if err := rows.Scan(&row.SourceID, &raw, pq.Array(&row.JSONNulls)); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &row.Fields); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func ExportBundle(ctx context.Context, tx *sql.Tx, processor string, sourceInputID int64) (Bundle, error) {
	contract, ok := processorContracts[processor]
	if !ok {
		return Bundle{}, fmt.Errorf("no sync contract for processor %s", processor)
	}
	bundle := Bundle{Rows: make(map[string][]PortableRow), ObjectOwners: make(map[int64]string)}
	exporter := bundleExporter{ctx: ctx, tx: tx, input: sourceInputID, bundle: bundle, cache: make(map[string]map[int64]PortableRow)}
	for _, step := range contract.steps {
		table := portableTables[step.table]
		predicate := matchesScope(table, step.where)
		var args []any
		if strings.Contains(predicate, "$1") {
			args = append(args, sourceInputID)
		}
		rows, err := queryContractRows(ctx, tx, table, predicate, args...)
		if err != nil {
			return Bundle{}, err
		}
		bundle.Rows[table.name] = rows
		for _, row := range rows {
			exporter.remember(table.name, row)
		}
	}
	for _, step := range contract.steps {
		for i := range bundle.Rows[step.table] {
			if err := exporter.enrich(step.table, &bundle.Rows[step.table][i]); err != nil {
				return Bundle{}, err
			}
		}
	}
	if err := exporter.collectRegistries(processor); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

type bundleExporter struct {
	ctx    context.Context
	tx     *sql.Tx
	input  int64
	bundle Bundle
	cache  map[string]map[int64]PortableRow
}

func (e *bundleExporter) remember(table string, row PortableRow) {
	if row.SourceID == 0 {
		return
	}
	if e.cache[table] == nil {
		e.cache[table] = make(map[int64]PortableRow)
	}
	e.cache[table][row.SourceID] = row
}

func (e *bundleExporter) enrich(name string, row *PortableRow) error {
	for _, ref := range portableTables[name].refs {
		id, err := readReferenceID(row.Fields[ref.column])
		if err != nil {
			return fmt.Errorf("export %s.%s: %w", name, ref.column, err)
		}
		if id == 0 {
			continue
		}
		key, err := e.referenceKey(ref.table, id)
		if err != nil {
			return fmt.Errorf("export %s.%s: %w", name, ref.column, err)
		}
		if row.ReferenceKeys == nil {
			row.ReferenceKeys = make(map[string]json.RawMessage)
		}
		row.ReferenceKeys[ref.column] = key
	}
	return nil
}

func (e *bundleExporter) referenceKey(name string, id int64) (json.RawMessage, error) {
	if name == "inputs" {
		var key string
		if id == e.input {
			key = "$input"
		} else if err := e.tx.QueryRowContext(e.ctx, "SELECT lower(btrim(md5)) FROM kb.inputs WHERE id=$1", id).Scan(&key); err != nil {
			return nil, fmt.Errorf("find source input %d: %w", id, err)
		}
		if key == "" {
			return nil, fmt.Errorf("no hash for source input %d", id)
		}
		return json.Marshal(key)
	}
	table := portableTables[name]
	row, known := e.cache[name][id]
	if !known {
		rows, err := queryContractRows(e.ctx, e.tx, table, sourceIDExpr(table)+" = $1", id)
		if err != nil {
			return nil, err
		}
		if len(rows) != 1 {
			return nil, fmt.Errorf("missing source %s row %d", name, id)
		}
		row = rows[0]
		e.remember(name, row)
		// Register the row before following cross-references, including cycles.
		if table.shared && !isVocabularyTable(name) {
			i := len(e.bundle.Rows[name])
			e.bundle.Rows[name] = append(e.bundle.Rows[name], row)
			if err := e.enrich(name, &row); err != nil {
				return nil, err
			}
			e.bundle.Rows[name][i] = row
		}
	}
	key := make(map[string]json.RawMessage)
	for _, col := range strings.Fields(table.key) {
		key[col] = row.Fields[col]
		for _, ref := range table.refs {
			if ref.column == col {
				target, err := readReferenceID(key[col])
				if err != nil {
					return nil, err
				}
				if target != 0 {
					key[col], err = e.referenceKey(ref.table, target)
					if err != nil {
						return nil, err
					}
				}
			}
		}
	}
	if len(key) == 0 {
		return nil, fmt.Errorf("no portable key for %s", name)
	}
	return json.Marshal(key)
}

func readReferenceID(raw json.RawMessage) (int64, error) {
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return 0, nil
	}
	return strconv.ParseInt(string(raw), 10, 64)
}

func ClearProcessor(ctx context.Context, tx *sql.Tx, _ string, processor string, localInputID int64) error {
	contract, ok := processorContracts[processor]
	if !ok {
		return fmt.Errorf("no sync contract for processor %s", processor)
	}
	var assertionIDs []int64
	if processor == "normalize_assertions" {
		var err error
		err = tx.QueryRowContext(ctx, "SELECT COALESCE(array_agg(t.id),ARRAY[]::bigint[]) FROM kb.semantic_assertions t WHERE "+assertionOwnership, localInputID).
			Scan(pq.Array(&assertionIDs))
		if err != nil {
			return fmt.Errorf("find owned assertions: %w", err)
		}
	}
	if processor == "associate_semantics" {
		// Scheduling state is local. Remove retry entries that reference the
		// outcomes this replacement will remove.
		if _, err := tx.ExecContext(ctx, `DELETE FROM kb.semantic_retry_queue WHERE outcome_id IN
			(SELECT id FROM kb.semantic_processing_outcomes WHERE input_record_id=$1)`, localInputID); err != nil {
			return fmt.Errorf("clear assertion retries: %w", err)
		}
	}
	for i := len(contract.steps) - 1; i >= 0; i-- {
		step := contract.steps[i]
		table := portableTables[step.table]
		if table.shared || table.name == "inputs" {
			continue
		}
		predicate := matchesScope(table, step.where)
		var scope any = localInputID
		switch table.name {
		case "semantic_assertions":
			predicate, scope = "t.id=ANY($1::bigint[])", pq.Array(assertionIDs)
		case "assertion_relations":
			predicate, scope = "t.assertion_id=ANY($1::bigint[]) OR t.related_assertion_id=ANY($1::bigint[])", pq.Array(assertionIDs)
		case "semantic_assertion_redirects":
			predicate, scope = "t.source_assertion_id=ANY($1::bigint[]) OR t.target_assertion_id=ANY($1::bigint[])", pq.Array(assertionIDs)
		}
		if predicate == "" {
			return fmt.Errorf("missing deletion scope for %s", step.table)
		}
		if table.id != "" {
			_, err := tx.ExecContext(ctx, "DELETE FROM kb.doc_sync_row_map m USING kb."+table.name+
				" t WHERE ("+predicate+") AND m.local_id="+sourceIDExpr(table)+" AND m.table_name=$2", scope, table.name)
			if err != nil {
				return fmt.Errorf("clear %s mapping: %w", step.table, err)
			}
		}
		if _, err := tx.ExecContext(ctx, "DELETE FROM kb."+table.name+" t WHERE "+predicate, scope); err != nil {
			return fmt.Errorf("clear %s: %w", step.table, err)
		}
	}
	return nil
}

type bundleImporter struct {
	ctx            context.Context
	tx             *sql.Tx
	site           string
	source, local  int64
	result         BundleResult
	deferred       []deferredRow
	objectOwners   map[int64]string
	resolvedOwners map[int64]int64
}

type deferredRow struct {
	table   tableContract
	id      int64
	row     PortableRow
	fields  map[string]json.RawMessage
	present bool
}

func ApplyBundle(ctx context.Context, tx *sql.Tx, site, processor string, sourceInputID, localInputID int64, bundle Bundle) (BundleResult, error) {
	contract, ok := processorContracts[processor]
	if !ok {
		return BundleResult{}, fmt.Errorf("no sync contract for processor %s", processor)
	}
	allowed := importSteps(processor, contract)
	for name, rows := range bundle.Rows {
		if !slices.Contains(allowed, name) {
			return BundleResult{}, fmt.Errorf("unexpected table %s in %s bundle", name, processor)
		}
		for _, row := range rows {
			if err := validatePortableRow(portableTables[name], row); err != nil {
				return BundleResult{}, err
			}
		}
	}
	for _, step := range contract.steps {
		rows, ok := bundle.Rows[step.table]
		if !ok {
			return BundleResult{}, fmt.Errorf("missing table %s in %s bundle", step.table, processor)
		}
		for _, row := range rows {
			if err := validateRowOwner(processor, step.table, row.Fields); err != nil {
				return BundleResult{}, err
			}
		}
	}
	if err := validateGovernedTerms(ctx, tx, bundle); err != nil {
		return BundleResult{}, err
	}
	if err := ClearProcessor(ctx, tx, site, processor, localInputID); err != nil {
		return BundleResult{}, err
	}
	im := bundleImporter{ctx: ctx, tx: tx, site: site, source: sourceInputID, local: localInputID,
		objectOwners: bundle.ObjectOwners, resolvedOwners: make(map[int64]int64)}
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(hashtextextended('docsync:portable-registry',0))"); err != nil {
		return BundleResult{}, err
	}
	for _, name := range allowed {
		for _, row := range bundle.Rows[name] {
			if err := im.apply(portableTables[name], row); err != nil {
				return BundleResult{}, fmt.Errorf("apply %s: %w", name, err)
			}
		}
	}
	for _, row := range im.deferred {
		if err := im.finish(row); err != nil {
			return BundleResult{}, fmt.Errorf("apply %s links: %w", row.table.name, err)
		}
	}
	return im.result, nil
}

func validatePortableRow(table tableContract, row PortableRow) error {
	if table.id != "" && row.SourceID <= 0 {
		return fmt.Errorf("%s row has invalid source ID", table.name)
	}
	expected := strings.Fields(table.fields)
	if len(row.Fields) != len(expected) {
		return fmt.Errorf("%s row does not match the portable contract", table.name)
	}
	for _, col := range expected {
		if !json.Valid(row.Fields[col]) {
			return fmt.Errorf("%s row has missing or invalid field %s", table.name, col)
		}
	}
	jsonColumns := strings.Fields(table.jsonFields)
	seenNulls := make(map[string]bool)
	for _, col := range row.JSONNulls {
		if seenNulls[col] || !slices.Contains(jsonColumns, col) || !bytes.Equal(row.Fields[col], []byte("null")) {
			return fmt.Errorf("%s row has invalid JSON null field %s", table.name, col)
		}
		seenNulls[col] = true
	}
	expectedRefs := 0
	for _, ref := range table.refs {
		id, err := readReferenceID(row.Fields[ref.column])
		if err != nil || id < 0 {
			return fmt.Errorf("%s row has invalid reference %s", table.name, ref.column)
		}
		if id != 0 {
			expectedRefs++
			if !json.Valid(row.ReferenceKeys[ref.column]) {
				return fmt.Errorf("%s row has no portable key for %s", table.name, ref.column)
			}
		}
	}
	if len(row.ReferenceKeys) != expectedRefs {
		return fmt.Errorf("%s row has unexpected reference keys", table.name)
	}
	return nil
}

func (im *bundleImporter) apply(table tableContract, row PortableRow) error {
	fields := cloneFields(row.Fields)
	for _, ref := range table.refs {
		if ref.deferred {
			delete(fields, ref.column)
		} else if err := im.rewriteReference(ref, row, fields); err != nil {
			return err
		}
	}
	if table.scope != "" && table.name != "inputs" {
		fields[table.scope] = json.RawMessage(strconv.FormatInt(im.local, 10))
	}
	if err := im.rewriteFields(table, fields, row); err != nil {
		return err
	}

	if table.name == "inputs" {
		return im.update(table, im.local, fields, row.JSONNulls)
	}

	localID, found, err := im.findShared(table, fields)
	if err != nil {
		return err
	}
	if !found {
		localID, err = im.insert(table, fields, row.JSONNulls)
		if err != nil && table.shared {
			var pqErr *pq.Error
			if !errors.As(err, &pqErr) || pqErr.Code != "23505" {
				return err
			}
			return fmt.Errorf("shared key conflicts with another local constraint on %s: %w", table.name, err)
		}
		if err != nil {
			return err
		}
		im.result.Rows++
	}
	if table.id != "" {
		var record any
		if !table.shared && table.name != "semantic_assertions" {
			record = im.local
		}
		_, err := im.tx.ExecContext(im.ctx, `INSERT INTO kb.doc_sync_row_map(site, table_name, source_id, local_id, local_input_id)
VALUES($1,$2,$3,$4,$5) ON CONFLICT(site,table_name,source_id)
DO UPDATE SET local_id=EXCLUDED.local_id, local_input_id=EXCLUDED.local_input_id`, im.site, table.name, row.SourceID, localID, record)
		if err != nil {
			return err
		}
	}
	if isVocabularyTable(table.name) || slices.ContainsFunc(table.refs, func(ref rowReference) bool { return ref.deferred }) {
		im.deferred = append(im.deferred, deferredRow{table: table, id: localID, row: row, fields: fields, present: found})
	}
	return nil
}

func fieldSubset(fields map[string]json.RawMessage, names string) map[string]json.RawMessage {
	values := make(map[string]json.RawMessage)
	for _, col := range strings.Fields(names) {
		if value, ok := fields[col]; ok {
			values[col] = value
		}
	}
	return values
}

func (im *bundleImporter) findShared(table tableContract, fields map[string]json.RawMessage) (id int64, found bool, err error) {
	switch {
	case table.shared, table.name == "semantic_assertions", table.name == "assertion_relations", table.name == "semantic_assertion_redirects":
	default:
		return 0, false, nil
	}
	query, raw, err := keyQuery(table, fields)
	if err != nil {
		return 0, false, err
	}
	err = im.tx.QueryRowContext(im.ctx, "SELECT "+sourceIDExpr(table)+query+" LIMIT 1", raw).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		switch {
		case table.name == "ontology_term_redirects" && bytes.Equal(fields["active"], []byte("true")):
			err = im.tx.QueryRowContext(im.ctx, "SELECT id FROM kb.ontology_term_redirects WHERE active AND source_term_id=$1",
				stringField(fields, "source_term_id")).Scan(&id)
		case table.name == "product_names" && stringField(fields, "status") == "proposed":
			err = im.tx.QueryRowContext(im.ctx, "SELECT id FROM kb.product_names WHERE status='proposed' AND product_name=$1",
				stringField(fields, "product_name")).Scan(&id)
		}
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
	}
	return id, err == nil, err
}

func keyQuery(table tableContract, fields map[string]json.RawMessage) (string, string, error) {
	var matches []string
	for _, col := range strings.Fields(table.key) {
		column := pq.QuoteIdentifier(col)
		if !json.Valid(fields[col]) {
			return "", "", fmt.Errorf("%s key has missing field %s", table.name, col)
		}
		if bytes.Equal(fields[col], []byte("null")) {
			matches = append(matches, "t."+column+" IS NULL")
		} else {
			matches = append(matches, "t."+column+" = x."+column)
		}
	}
	if len(matches) == 0 {
		return "", "", fmt.Errorf("missing portable key for %s", table.name)
	}
	raw, err := json.Marshal(fieldSubset(fields, table.key))
	query := " FROM kb." + table.name + " t CROSS JOIN jsonb_populate_record(NULL::kb." +
		table.name + ",$1::jsonb) x WHERE " + strings.Join(matches, " AND ")
	return query, string(raw), err
}

func (im *bundleImporter) rewriteReference(ref rowReference, row PortableRow, fields map[string]json.RawMessage) error {
	sourceID, err := readReferenceID(row.Fields[ref.column])
	if err != nil || sourceID == 0 {
		fields[ref.column] = row.Fields[ref.column]
		return err
	}
	id, err := im.lookup(ref.table, sourceID, row.ReferenceKeys[ref.column])
	if err != nil {
		return fmt.Errorf("%s: %w", ref.column, err)
	}
	fields[ref.column] = json.RawMessage(strconv.FormatInt(id, 10))
	return nil
}

func (im *bundleImporter) lookup(name string, sourceID int64, raw json.RawMessage) (int64, error) {
	if name == "inputs" {
		var key string
		if err := json.Unmarshal(raw, &key); err != nil {
			return 0, err
		}
		if key == "$input" && sourceID == im.source {
			return im.local, nil
		}
		var id int64
		if sourceID > 0 {
			err := im.tx.QueryRowContext(im.ctx, "SELECT input_id FROM kb.doc_sync_inputs WHERE site=$1 AND source_id=$2 AND md5=$3",
				im.site, sourceID, key).Scan(&id)
			if !errors.Is(err, sql.ErrNoRows) {
				return id, err
			}
		}
		var count int
		if err := im.tx.QueryRowContext(im.ctx, "SELECT COALESCE(min(id),0), count(*) FROM kb.inputs WHERE md5 IS NOT NULL AND lower(btrim(md5))=$1", key).Scan(&id, &count); err != nil {
			return 0, fmt.Errorf("resolve input dependency %d: %w", sourceID, err)
		}
		if count != 1 {
			return 0, fmt.Errorf("input dependency %d has %d local matches", sourceID, count)
		}
		return id, nil
	}
	table := portableTables[name]
	if sourceID != 0 {
		var id int64
		err := im.tx.QueryRowContext(im.ctx, "SELECT m.local_id FROM kb.doc_sync_row_map m JOIN kb."+name+
			" t ON "+sourceIDExpr(table)+"=m.local_id WHERE m.site=$1 AND m.table_name=$2 AND m.source_id=$3",
			im.site, name, sourceID).Scan(&id)
		if err == nil {
			return id, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	}
	var key map[string]json.RawMessage
	if err := json.Unmarshal(raw, &key); err != nil {
		return 0, err
	}
	for _, ref := range table.refs {
		if val := key[ref.column]; len(val) != 0 && (val[0] == '{' || val[0] == '"') {
			id, err := im.lookup(ref.table, 0, val)
			if err != nil {
				return 0, err
			}
			key[ref.column] = json.RawMessage(strconv.FormatInt(id, 10))
		}
	}
	if err := im.rewriteFields(table, key, PortableRow{}); err != nil {
		return 0, err
	}
	query, args, err := keyQuery(table, key)
	if err != nil {
		return 0, err
	}
	if table.scope != "" {
		query += " AND t." + pq.QuoteIdentifier(table.scope) + " = $2"
	}
	values := []any{args}
	if table.scope != "" {
		values = append(values, im.local)
	}
	var id int64
	err = im.tx.QueryRowContext(im.ctx, "SELECT "+sourceIDExpr(table)+query+" LIMIT 1", values...).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("resolve %s dependency %d: %w", name, sourceID, err)
	}
	return id, nil
}

func insertValues(columns, nulls []string) string {
	values := make([]string, len(columns))
	for i, col := range columns {
		if slices.Contains(nulls, col) {
			values[i] = "'null'::jsonb"
		} else {
			values[i] = pq.QuoteIdentifier(col)
		}
	}
	return strings.Join(values, ",")
}

func (im *bundleImporter) insert(table tableContract, fields map[string]json.RawMessage, nulls []string) (int64, error) {
	var columns []string
	for name := range fields {
		columns = append(columns, name)
	}
	slices.Sort(columns)
	cols := sqlFields(strings.Join(columns, " "))
	values := insertValues(columns, nulls)
	if table.name == "ontology_term_headers" || table.name == "ontology_term_revisions" {
		cols += ",create_time"
		values += ",now()"
	}
	if table.name == "ontology_term_revisions" {
		cols += ",modify_time"
		values += ",now()"
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return 0, err
	}
	stmt := "INSERT INTO kb." + table.name + "(" + cols + ") SELECT " + values +
		" FROM jsonb_populate_record(NULL::kb." + table.name + ",$1::jsonb)"
	if table.id == "" {
		_, err := im.tx.ExecContext(im.ctx, stmt, string(raw))
		return 0, err
	}
	var id int64
	err = im.tx.QueryRowContext(im.ctx, stmt+" RETURNING "+pq.QuoteIdentifier(table.id), string(raw)).Scan(&id)
	return id, err
}

func (im *bundleImporter) update(table tableContract, id int64, fields map[string]json.RawMessage, nulls []string) error {
	if len(fields) == 0 {
		return nil
	}
	var columns []string
	for name := range fields {
		columns = append(columns, name)
	}
	slices.Sort(columns)
	cols := sqlFields(strings.Join(columns, " "))
	raw, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	stmt := "UPDATE kb." + table.name + " t SET (" + cols + ") = (SELECT " + insertValues(columns, nulls) +
		" FROM jsonb_populate_record(NULL::kb." + table.name + ",$1::jsonb)) WHERE t." + pq.QuoteIdentifier(table.id) + "=$2"
	if table.id == "" {
		stmt = "UPDATE kb." + table.name + " t SET (" + cols + ") = (SELECT " + insertValues(columns, nulls) +
			" FROM jsonb_populate_record(NULL::kb." + table.name + ",$1::jsonb)) WHERE t.term_id=$2"
		var term string
		if err := json.Unmarshal(fields["term_id"], &term); err != nil {
			return err
		}
		_, err = im.tx.ExecContext(im.ctx, stmt, string(raw), term)
		return err
	}
	_, err = im.tx.ExecContext(im.ctx, stmt, string(raw), id)
	return err
}

func (im *bundleImporter) finish(work deferredRow) error {
	changes := fieldSubset(work.fields, work.table.key)
	for _, ref := range work.table.refs {
		if !ref.deferred {
			continue
		}
		if err := im.rewriteReference(ref, work.row, changes); err != nil {
			return err
		}
	}
	if !work.present {
		if len(changes) == len(strings.Fields(work.table.key)) {
			return nil
		}
		for col := range changes {
			work.fields[col] = changes[col]
		}
		return im.update(work.table, work.id, work.fields, work.row.JSONNulls)
	}
	if !isVocabularyTable(work.table.name) {
		return nil
	}
	for col, value := range changes {
		work.fields[col] = value
	}
	raw, err := json.Marshal(work.fields)
	if err != nil {
		return err
	}
	query, keys, err := keyQuery(work.table, work.fields)
	if err != nil {
		return err
	}
	var whereArg any = keys
	if work.table.id != "" {
		query = " FROM kb." + work.table.name + " t WHERE " + sourceIDExpr(work.table) + "=$1"
		whereArg = work.id
	}
	var differs bool
	if err := im.tx.QueryRowContext(im.ctx, "SELECT ("+rowJSON(work.table.fields)+") <> $2::jsonb"+query, whereArg, string(raw)).Scan(&differs); err != nil {
		return err
	}
	if differs {
		im.result.Conflicts = append(im.result.Conflicts, work.table.name+" "+keys)
	}
	return nil
}
