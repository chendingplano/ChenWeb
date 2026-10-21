"""Offline, exact-evidence summary benchmark. Python standard library only."""

import argparse
from collections import defaultdict
import hashlib
import json
import logging
import math
from pathlib import Path
import sys

ROOT = Path(__file__).resolve().parent
DATASET = ROOT / 'datasets/1.0.0/benchmark.json'
LOG = logging.getLogger('summary-benchmark.20261021-741')


def digest(data):
    payload = json.dumps(data, ensure_ascii=False, sort_keys=True, separators=(',', ':'))
    return hashlib.sha256(payload.encode('utf-8')).hexdigest()


def char_count(text):
    return sum(not c.isspace() for c in text)


def normalized(text):
    return ' '.join(text.split())


def require(condition, message):
    if not condition:
        raise ValueError(message)


def span_text(lines, span):
    require(isinstance(span, dict) and set(span) == {'line', 'start', 'end'}, 'invalid span fields')
    require(all(type(span[k]) is int for k in span), 'span coordinates must be integers')
    require(span['line'] in lines, 'unknown source line')
    text = lines[span['line']]['text']
    require(0 <= span['start'] < span['end'] <= len(text), 'span outside source text')
    return text[span['start']:span['end']]


def covers(have, need):
    # A required span must survive intact in a single verified source span.
    return any(s['line'] == need['line'] and s['start'] <= need['start']
               and s['end'] >= need['end'] for s in have)


def gold_spans(case):
    # First alternative is the canonical feasible reference, not a candidate oracle.
    spans = [s for u in case['units'] for s in u['evidence'][0]]
    by_line = defaultdict(list)
    for s in spans:
        by_line[s['line']].append((s['start'], s['end']))
    result = []
    for line in case['lines']:
        merged = []
        for start, end in sorted(by_line[line['id']]):
            if merged and start <= merged[-1][1]:
                merged[-1][1] = max(merged[-1][1], end)
            else:
                merged.append([start, end])
        result.extend({'line': line['id'], 'start': a, 'end': b} for a, b in merged)
    return result


def validate_dataset(data):
    require(data['schema_version'] == 1 and data['version'] and data['dataset_id'], 'invalid dataset version')
    require(data['cases'], 'empty dataset')
    ids = set()
    for case in data['cases']:
        require(case['id'] not in ids, 'duplicate case ID')
        ids.add(case['id'])
        require(case['language'] in {'en', 'zh-cn', 'mixed'}, 'unsupported language')
        require(case['stratum'] in {'diagnostic', 'real-pilot'}, 'unsupported stratum')
        require(type(case['budget_chars']) is int and case['budget_chars'] > 0, 'invalid budget')
        require(case['tags'] and case['source']['kind'] and case['source']['description'], 'missing provenance')
        require(case['lines'] and case['units'], 'empty case')
        lines = {line['id']: line for line in case['lines']}
        require(len(lines) == len(case['lines']), 'duplicate source line')
        require(list(lines) == sorted(lines), 'source lines must be ordered')
        for line in case['lines']:
            require(type(line['id']) is int and line['id'] > 0 and line['kind'], 'invalid line metadata')
            require(isinstance(line['text'], str) and line['text'].strip(), 'empty source line')
        unit_ids = set()
        for unit in case['units']:
            require(unit['id'] not in unit_ids, 'duplicate unit ID')
            unit_ids.add(unit['id'])
            require(type(unit['weight']) in {int, float} and math.isfinite(unit['weight'])
                    and unit['weight'] > 0, 'invalid weight')
            require(type(unit['critical']) is bool and unit['rationale'] and unit['kind'], 'invalid unit')
            require(unit['evidence'], 'missing evidence alternatives')
            for alternative in unit['evidence']:
                require(alternative, 'empty evidence alternative')
                for span in alternative:
                    require(span_text(lines, span).strip(), 'whitespace-only evidence')
        reference_size = sum(char_count(span_text(lines, s)) for s in gold_spans(case))
        require(reference_size <= case['budget_chars'], f"{case['id']}: gold exceeds budget")
        require(reference_size < sum(char_count(x['text']) for x in case['lines']),
                f"{case['id']}: case has no non-gold source content")
    return data


def load_dataset(path=DATASET):
    path = Path(path)
    data = validate_dataset(json.loads(path.read_text(encoding='utf-8')))
    lock = path.with_name('freeze.json')
    require(lock.exists(), 'dataset is not frozen: missing freeze.json')
    require(json.loads(lock.read_text(encoding='utf-8'))['dataset_sha256'] == digest(data),
            'dataset changed after freeze; create a new version')
    for case in data['cases']:
        source = case['source']
        if source['kind'] != 'canonical-excerpt':
            continue
        snapshot = (path.parent / source['snapshot']).resolve()
        require(snapshot.is_relative_to(path.parent.resolve()), 'source snapshot outside dataset')
        raw = snapshot.read_bytes()
        require(hashlib.sha256(raw).hexdigest() == source['snapshot_sha256'], 'source snapshot digest mismatch')
        canonical = {}
        for row in raw.decode('utf-8').splitlines():
            fields = row.split('\t', 6)
            require(len(fields) == 7, 'invalid canonical source row')
            canonical[int(fields[0])] = fields[6]
        for line in case['lines']:
            require(canonical.get(line['id']) == line['text'], 'excerpt differs from canonical snapshot')
    return data


def score_case(case, prediction):
    lines = {line['id']: line for line in case['lines']}
    segments = prediction.get('segments', []) if prediction else []
    require(isinstance(segments, list), 'segments must be a list')
    verified = []
    errors = []
    valid_segments = 0
    rendered = []
    previous = None
    ordered = True
    total_span_chars = 0
    positions = set()
    for index, segment in enumerate(segments):
        require(isinstance(segment, dict) and isinstance(segment.get('text'), str), 'invalid segment text')
        rendered.append(segment['text'])
        try:
            require(set(segment) == {'text', 'spans'}, 'unsupported segment fields')
            spans = segment['spans']
            require(isinstance(spans, list) and spans, 'missing source spans')
            pieces = [span_text(lines, s) for s in spans]
            require(segment['text'].strip() and normalized(segment['text']) == normalized(' '.join(pieces)),
                    'text does not match cited source spans')
            for span in spans:
                key = (span['line'], span['start'])
                if previous is not None and key < previous:
                    ordered = False
                previous = (span['line'], span['end'])
                total_span_chars += span['end'] - span['start']
                positions.update((span['line'], n) for n in range(span['start'], span['end']))
            verified.extend(spans)
            valid_segments += 1
        except (ValueError, KeyError, TypeError) as exc:
            errors.append(f'segment {index}: {exc}')
    covered = []
    for unit in case['units']:
        if any(all(covers(verified, span) for span in alternative) for alternative in unit['evidence']):
            covered.append(unit['id'])
    weights = sum(u['weight'] for u in case['units'])
    hit_weight = sum(u['weight'] for u in case['units'] if u['id'] in covered)
    critical = [u for u in case['units'] if u['critical']]
    critical_recall = (sum(u['id'] in covered for u in critical) / len(critical)) if critical else None
    size = char_count('\n'.join(rendered))
    nonempty = bool(size)
    within_budget = size <= case['budget_chars']
    citation_validity = valid_segments / len(segments) if segments else 0.0
    duplicate_rate = 1 - len(positions) / total_span_chars if total_span_chars else 0.0
    gates = {
        'present': prediction is not None, 'nonempty': nonempty,
        'within_budget': within_budget, 'valid_citations': bool(segments) and not errors,
        'source_order': ordered, 'no_duplicate_spans': duplicate_rate == 0,
        'critical_complete': critical_recall is None or critical_recall == 1,
    }
    return {
        'case_id': case['id'], 'language': case['language'], 'stratum': case['stratum'],
        'tags': case['tags'], 'status': 'scored' if prediction else 'missing',
        'weighted_coverage': hit_weight / weights, 'critical_recall': critical_recall,
        'citation_validity': citation_validity, 'duplicate_span_rate': duplicate_rate,
        'output_chars': size, 'budget_chars': case['budget_chars'], 'within_budget': within_budget,
        'covered_units': covered, 'missing_units': [u['id'] for u in case['units'] if u['id'] not in covered],
        'accepted': all(gates.values()), 'gates': gates, 'citation_errors': errors,
        'measurement': prediction.get('measurement') if prediction else None,
    }


def aggregate(rows):
    def mean(key):
        values = [r[key] for r in rows if r[key] is not None]
        return sum(values) / len(values) if values else None
    return {'case_count': len(rows), 'weighted_coverage': mean('weighted_coverage'),
            'critical_recall': mean('critical_recall'), 'citation_validity': mean('citation_validity'),
            'accepted_rate': sum(r['accepted'] for r in rows) / len(rows)}


def evaluate(data, submission):
    validate_dataset(data)
    require(submission['schema_version'] == 1, 'unsupported submission schema')
    require(submission['dataset_sha256'] == digest(data), 'submission dataset digest mismatch')
    system = submission['system']
    require(system['name'] and system['revision'] and isinstance(system['configuration'], dict),
            'system name, revision and configuration required')
    allowed = {'case_id', 'segments', 'measurement'}
    known = {c['id'] for c in data['cases']}
    predictions = {}
    for prediction in submission['predictions']:
        require(set(prediction) <= allowed and {'case_id', 'segments'} <= set(prediction),
                'unsupported prediction fields (all output belongs in segments)')
        case_id = prediction['case_id']
        require(case_id in known and case_id not in predictions, 'unknown or duplicate case ID')
        predictions[case_id] = prediction
        measurement = prediction.get('measurement')
        if measurement is not None:
            require(isinstance(measurement, dict) and measurement.get('scope')
                    and measurement.get('machine') and measurement.get('repetitions'), 'measurement provenance missing')
            require(type(measurement['repetitions']) is int and measurement['repetitions'] > 0,
                    'invalid measurement repetitions')
            for field in ('wall_ms', 'peak_rss_bytes', 'cost_usd'):
                value = measurement.get(field)
                require(value is None or (type(value) in {int, float} and math.isfinite(value) and value >= 0),
                        f'invalid measurement {field}')
    rows = [score_case(c, predictions.get(c['id'])) for c in data['cases']]
    groups = {}
    for field in ('stratum', 'language', 'tags'):
        bucket = defaultdict(list)
        for row in rows:
            for value in row[field] if field == 'tags' else [row[field]]:
                bucket[value].append(row)
        groups[field] = {key: aggregate(value) for key, value in sorted(bucket.items())}
    return {'schema_version': 1, 'dataset_sha256': digest(data), 'system': system,
            'score_kind': 'exact_extract_evidence_coverage_not_semantic_quality',
            'overall': aggregate(rows), 'groups': groups, 'cases': rows}


def baseline(data, method):
    predictions = []
    for case in data['cases']:
        lines = {line['id']: line for line in case['lines']}
        if method == 'gold-control':
            spans = gold_spans(case)
        else:
            order = case['lines']
            if method == 'structural':
                # One content line after each heading, then remaining lines; no gold access.
                preferred = [order[i + 1] for i, line in enumerate(order[:-1])
                             if line['kind'] == 'heading' and order[i + 1]['kind'] != 'heading']
                used = {line['id'] for line in preferred}
                order = preferred + [line for line in order if line['id'] not in used]
            spans = []
            remaining = case['budget_chars']
            for line in order:
                size = char_count(line['text'])
                if size <= remaining:
                    spans.append({'line': line['id'], 'start': 0, 'end': len(line['text'])})
                    remaining -= size
            spans.sort(key=lambda s: (s['line'], s['start']))
        predictions.append({'case_id': case['id'], 'segments': [
            {'text': span_text(lines, s), 'spans': [s]} for s in spans]})
    return {'schema_version': 1, 'dataset_sha256': digest(data),
            'system': {'name': method, 'revision': '1.0.0', 'configuration': {'budget': 'case.budget_chars'},
                       'purpose': 'harness control; not a candidate result'}, 'predictions': predictions}


def candidate_inputs(data):
    return {'schema_version': 1, 'dataset_sha256': digest(data),
            'budget_unit': 'non-whitespace Unicode code points',
            'cases': [{key: case[key] for key in ('id', 'language', 'budget_chars', 'lines')}
                      for case in data['cases']]}


def stress_input(sentences, chunk_sentences):
    require(type(sentences) is int and 1 <= sentences <= 100000, 'sentences must be 1..100000')
    require(chunk_sentences is None or (type(chunk_sentences) is int and chunk_sentences > 0),
            'chunk size must be positive')
    topics = ['temperature', 'pressure', 'voltage', 'current', 'moisture', 'flow',
              'speed', 'storage', 'inspection', 'cooling', 'transport', 'maintenance']
    text = [f"For inspection item {i + 1}, the {topics[i % len(topics)]} control shall be checked before operation."
            for i in range(sentences)]
    size = chunk_sentences or sentences
    inputs = [{'input_id': f'input-{i // size + 1:05d}', 'sentences': text[i:i + size]}
              for i in range(0, sentences, size)]
    return {'schema_version': 1, 'fixture': 'summary-scale-v1', 'quality_labels': None,
            'purpose': 'scaling only; generated repetitions do not measure summary quality',
            'scope': 'chunk' if chunk_sentences else 'document', 'sentence_count': sentences,
            'chunk_sentences': chunk_sentences, 'inputs': inputs, 'source_sha256': digest(text)}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--dataset', type=Path, default=DATASET)
    sub = parser.add_subparsers(dest='command', required=True)
    sub.add_parser('validate')
    inputs = sub.add_parser('inputs')
    inputs.add_argument('--output', type=Path, required=True)
    base = sub.add_parser('baseline')
    base.add_argument('--method', choices=['lead', 'structural', 'gold-control'], required=True)
    base.add_argument('--output', type=Path, required=True)
    score = sub.add_parser('score')
    score.add_argument('submission', type=Path)
    score.add_argument('--output', type=Path)
    stress = sub.add_parser('stress')
    stress.add_argument('--sentences', type=int, required=True)
    stress.add_argument('--chunk-sentences', type=int)
    stress.add_argument('--output', type=Path, required=True)
    args = parser.parse_args(argv)
    logging.basicConfig(level=logging.INFO, format='%(name)s: %(message)s')
    try:
        if args.command == 'stress':
            result = stress_input(args.sentences, args.chunk_sentences)
        else:
            data = load_dataset(args.dataset)
        if args.command == 'validate':
            result = {'valid': True, 'cases': len(data['cases']), 'dataset_sha256': digest(data)}
        elif args.command == 'baseline':
            result = baseline(data, args.method)
        elif args.command == 'inputs':
            result = candidate_inputs(data)
        elif args.command == 'score':
            result = evaluate(data, json.loads(args.submission.read_text(encoding='utf-8')))
        payload = json.dumps(result, ensure_ascii=False, indent=2, allow_nan=False) + '\n'
        output = getattr(args, 'output', None)
        if output:
            output.write_text(payload, encoding='utf-8')
            LOG.info('%s: wrote %s', args.command, output)
        else:
            print(payload, end='')
        return 0
    except (ValueError, KeyError, TypeError, OSError) as exc:
        LOG.error('%s', exc)
        return 1


if __name__ == '__main__':
    sys.exit(main())
