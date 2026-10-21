import copy
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import shutil
import unittest

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))


class BenchmarkTests(unittest.TestCase):
    def setUp(self):
        self.assertIsNotNone(importlib.util.find_spec('benchmark'), 'benchmark harness is missing')
        import benchmark
        self.b = benchmark
        self.case = {
            'id': 'exception', 'language': 'en', 'stratum': 'diagnostic',
            'tags': ['exception'], 'budget_chars': 100,
            'source': {'kind': 'synthetic', 'description': 'Test source'},
            'lines': [
                {'id': 1, 'kind': 'paragraph', 'text': 'The device shall remain below 8°C.'},
                {'id': 2, 'kind': 'paragraph', 'text': 'Except during maintenance.'},
                {'id': 3, 'kind': 'paragraph', 'text': 'General background.'},
            ],
            'units': [{'id': 'limit', 'kind': 'requirement', 'weight': 2,
                       'critical': True, 'rationale': 'Limit and exception are inseparable.',
                       'evidence': [[self.span(1), self.span(2)]]}],
        }

    def span(self, line):
        text = {1: 'The device shall remain below 8°C.', 2: 'Except during maintenance.',
                3: 'General background.'}[line]
        return {'line': line, 'start': 0, 'end': len(text)}

    def prediction(self, *lines):
        source = {x['id']: x['text'] for x in self.case['lines']}
        return {'case_id': self.case['id'], 'segments': [
            {'text': source[n], 'spans': [self.span(n)]} for n in lines]}

    def dataset(self):
        return {'schema_version': 1, 'dataset_id': 'test', 'version': '1.0.0',
                'cases': [self.case]}

    def submission(self, predictions):
        return {'schema_version': 1, 'dataset_sha256': self.b.digest(self.dataset()),
                'system': {'name': 'test', 'revision': '1', 'configuration': {}},
                'predictions': predictions}

    def test_complete_evidence_gets_full_credit(self):
        result = self.b.score_case(self.case, self.prediction(1, 2))
        self.assertEqual(result['weighted_coverage'], 1)
        self.assertEqual(result['critical_recall'], 1)
        self.assertTrue(result['accepted'])

    def test_omitted_exception_gets_no_credit(self):
        result = self.b.score_case(self.case, self.prediction(1))
        self.assertEqual(result['weighted_coverage'], 0)
        self.assertEqual(result['missing_units'], ['limit'])
        self.assertFalse(result['accepted'])

    def test_changed_number_with_correct_citation_gets_no_credit(self):
        p = self.prediction(1, 2)
        p['segments'][0]['text'] = p['segments'][0]['text'].replace('8°C', '80°C')
        result = self.b.score_case(self.case, p)
        self.assertEqual(result['weighted_coverage'], 0)
        self.assertLess(result['citation_validity'], 1)
        self.assertFalse(result['accepted'])

    def test_citing_whole_document_without_quoting_it_gets_no_credit(self):
        p = self.prediction(3)
        p['segments'][0]['spans'] = [self.span(1), self.span(2), self.span(3)]
        self.assertEqual(self.b.score_case(self.case, p)['weighted_coverage'], 0)

    def test_over_budget_is_ineligible_even_with_full_coverage(self):
        self.case['budget_chars'] = 5
        result = self.b.score_case(self.case, self.prediction(1, 2))
        self.assertEqual(result['weighted_coverage'], 1)
        self.assertFalse(result['accepted'])
        self.assertFalse(result['within_budget'])

    def test_duplicates_do_not_increase_coverage(self):
        result = self.b.score_case(self.case, self.prediction(1, 2, 1, 2))
        self.assertEqual(result['weighted_coverage'], 1)
        self.assertEqual(result['duplicate_span_rate'], 0.5)

    def test_bad_span_is_rejected(self):
        p = self.prediction(1)
        p['segments'][0]['spans'][0]['end'] = 99999
        result = self.b.score_case(self.case, p)
        self.assertEqual(result['citation_validity'], 0)
        self.assertFalse(result['accepted'])

    def test_malformed_empty_and_noninteger_spans_are_rejected(self):
        for change in ({'start': 0, 'end': 0}, {'start': True}, {'end': 1.5}, {'line': 999}):
            p = self.prediction(1, 2)
            p['segments'][0]['spans'][0].update(change)
            self.assertFalse(self.b.score_case(self.case, p)['accepted'])

    def test_reordered_evidence_is_ineligible(self):
        result = self.b.score_case(self.case, self.prediction(2, 1))
        self.assertEqual(result['weighted_coverage'], 1)
        self.assertFalse(result['gates']['source_order'])
        self.assertFalse(result['accepted'])

    def test_uncited_extra_text_cannot_hide_outside_budget(self):
        p = self.prediction(1, 2)
        p['segments'][0]['text'] = 'Unsupported heading ' * 10 + p['segments'][0]['text']
        result = self.b.score_case(self.case, p)
        self.assertFalse(result['within_budget'])
        self.assertFalse(result['accepted'])

    def test_whitespace_only_output_is_ineligible(self):
        p = self.prediction(1)
        p['segments'][0]['text'] = ' \n '
        self.assertFalse(self.b.score_case(self.case, p)['accepted'])

    def test_unknown_output_fields_are_rejected(self):
        p = self.prediction(1, 2)
        p['summary'] = 'Uncounted output'
        with self.assertRaises(ValueError):
            self.b.evaluate(self.dataset(), self.submission([p]))

    def test_partial_sentence_cannot_drop_negation(self):
        p = self.prediction(1, 2)
        p['segments'][0]['spans'][0]['start'] = 10
        p['segments'][0]['text'] = self.case['lines'][0]['text'][10:]
        self.assertEqual(self.b.score_case(self.case, p)['weighted_coverage'], 0)

    def test_character_budget_counts_chinese_and_ignores_whitespace(self):
        self.assertEqual(self.b.char_count('温度 ≤ 8℃\n保存'), 7)

    def test_missing_case_counts_zero_and_fails(self):
        result = self.b.evaluate(self.dataset(), self.submission([]))
        self.assertEqual(result['overall']['weighted_coverage'], 0)
        self.assertEqual(result['overall']['accepted_rate'], 0)
        self.assertEqual(result['cases'][0]['status'], 'missing')

    def test_unknown_or_duplicate_case_is_error(self):
        p = self.prediction(1, 2)
        with self.assertRaises(ValueError):
            self.b.evaluate(self.dataset(), self.submission([p, p]))
        p['case_id'] = 'unknown'
        with self.assertRaises(ValueError):
            self.b.evaluate(self.dataset(), self.submission([p]))

    def test_digest_mismatch_is_error(self):
        p = self.submission([])
        p['dataset_sha256'] = 'wrong'
        with self.assertRaises(ValueError):
            self.b.evaluate(self.dataset(), p)

    def test_absent_measurements_are_not_zero_cost(self):
        result = self.b.evaluate(self.dataset(), self.submission([self.prediction(1, 2)]))
        self.assertIsNone(result['cases'][0]['measurement'])

    def test_invalid_gold_is_error(self):
        data = self.dataset()
        data['cases'][0]['units'][0]['evidence'][0][0]['line'] = 999
        with self.assertRaises(ValueError):
            self.b.validate_dataset(data)

    def test_infeasible_gold_budget_is_error(self):
        self.case['budget_chars'] = 1
        with self.assertRaises(ValueError):
            self.b.validate_dataset(self.dataset())

    def test_fixture_lock_and_gold_control(self):
        data = self.b.load_dataset()
        self.b.validate_dataset(data)
        lock = json.loads((ROOT / 'datasets/1.0.0/freeze.json').read_text())
        self.assertEqual(lock['dataset_sha256'], self.b.digest(data))
        result = self.b.evaluate(data, self.b.baseline(data, 'gold-control'))
        self.assertEqual(result['overall']['weighted_coverage'], 1)
        self.assertEqual(result['overall']['accepted_rate'], 1)

    def test_fixture_has_independent_languages_and_real_pilot(self):
        data = self.b.load_dataset()
        self.assertGreaterEqual(len(data['cases']), 12)
        self.assertTrue({'en', 'zh-cn', 'mixed'} <= {c['language'] for c in data['cases']})
        self.assertTrue({'diagnostic', 'real-pilot'} <= {c['stratum'] for c in data['cases']})
        required = {'exception', 'table', 'definition', 'negation', 'near-duplicate',
                    'cross-line', 'ocr', 'overview', 'short-line'}
        self.assertTrue(required <= {t for c in data['cases'] for t in c['tags']})

    def test_edited_real_source_snapshot_is_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            target = Path(tmp) / 'dataset'
            shutil.copytree(ROOT / 'datasets/1.0.0', target)
            source = target / 'sources/record-416.lines.txt'
            source.write_text(source.read_text() + '\nmodified\n')
            with self.assertRaises(ValueError):
                self.b.load_dataset(target / 'benchmark.json')

    def test_stress_input_is_scoped_to_document_or_chunks(self):
        whole = self.b.stress_input(1000, None)
        chunks = self.b.stress_input(1000, 100)
        self.assertEqual(len(whole['inputs']), 1)
        self.assertEqual(len(chunks['inputs']), 10)
        self.assertEqual(whole['inputs'][0]['sentences'],
                         [s for chunk in chunks['inputs'] for s in chunk['sentences']])
        self.assertEqual(whole, self.b.stress_input(1000, None))
        with self.assertRaises(ValueError):
            self.b.stress_input(0, 100)

    def test_table_values_without_treatment_scope_get_no_credit(self):
        data = json.loads((ROOT / 'datasets/1.0.0/benchmark.json').read_text())
        candidate = self.b.baseline(data, 'gold-control')['predictions'][-1]
        candidate['segments'] = [s for s in candidate['segments'] if s['spans'][0]['line'] != 128]
        result = self.b.score_case(data['cases'][-1], candidate)
        self.assertEqual(result['weighted_coverage'], 0)
        self.assertEqual(result['critical_recall'], 0)

    def test_candidate_inputs_do_not_include_gold(self):
        inputs = self.b.candidate_inputs(self.dataset())
        self.assertEqual(inputs['dataset_sha256'], self.b.digest(self.dataset()))
        self.assertEqual(set(inputs['cases'][0]), {'id', 'language', 'budget_chars', 'lines'})


if __name__ == '__main__':
    unittest.main()
