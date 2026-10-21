import importlib.util
from pathlib import Path
import unittest

WORKER = Path(__file__).resolve().parents[2] / 'server/api/doc-processing/sumy_worker.py'


class WorkerTests(unittest.TestCase):
    def setUp(self):
        self.assertTrue(WORKER.exists(), 'Sumy worker is missing')
        spec = importlib.util.spec_from_file_location('sumy_worker', WORKER)
        self.worker = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.worker)

    def run_chunk(self, texts, **kwargs):
        chunk = {'seq_no': 7, 'lines': [
            {'line_number': i + 20, 'content': t, 'line_type': 'text'}
            for i, t in enumerate(texts)]}
        result = self.worker.summarize({'algorithm': kwargs.get('algorithm', 'lexrank'),
                                       'sentence_count': kwargs.get('sentence_count', 3),
                                       'language': kwargs.get('language', 'en'), 'chunks': [chunk]})
        return chunk, result['summaries'][0]

    def assert_grounded(self, chunk, result):
        lines = {x['line_number']: x['content'] for x in chunk['lines']}
        for segment in result['sentences']:
            pieces = [lines[s['line']][s['start']:s['end']] for s in segment['spans']]
            self.assertEqual(' '.join(segment['text'].split()), ' '.join(' '.join(pieces).split()))
        self.assertEqual(result['summary_text'], '\n'.join(s['text'] for s in result['sentences']))

    def test_english_all_algorithms_are_grounded(self):
        for algorithm in ('lexrank', 'textrank', 'lsa', 'luhn', 'sumbasic'):
            chunk, result = self.run_chunk([
                'The equipment supports storage and transport.',
                'Storage temperature shall not exceed 8°C.',
                'The limit does not apply during the proof test.',
                'Operators shall record the storage temperature.',
                'The handbook illustrates the storage containers.',
            ], algorithm=algorithm, sentence_count=2)
            self.assertEqual(result['seq_no'], 7)
            self.assertEqual(len(result['sentences']), 2)
            self.assert_grounded(chunk, result)

    def test_chinese_offsets_are_codepoints(self):
        chunk, result = self.run_chunk(['防护门打开时，不得启动电机。', '不得绕过安全联锁。',
                                      '设备应保持清洁。'], language='zh-cn', sentence_count=2)
        self.assertEqual(result['language'], 'zh')
        self.assert_grounded(chunk, result)
        self.assertEqual(len(result['sentences']), 2)

    def test_repeated_sentences_map_to_distinct_occurrences(self):
        chunk, result = self.run_chunk(['Pressure shall remain below 8 kPa.',
                                      'Pressure shall remain below 8 kPa.'], sentence_count=5)
        self.assert_grounded(chunk, result)
        self.assertEqual([s['spans'][0]['line'] for s in result['sentences']], [20, 21])

    def test_sentence_split_across_lines_retains_both_spans(self):
        chunk, result = self.run_chunk(['If analysis is delayed, store the extract',
                                      'at 0°C to 4°C for no longer than 48 h.'], sentence_count=3)
        self.assertEqual(len(result['sentences']), 1)
        self.assertEqual([s['line'] for s in result['sentences'][0]['spans']], [20, 21])
        self.assert_grounded(chunk, result)

    def test_uppercase_and_short_lines_are_not_discarded(self):
        chunk, result = self.run_chunk(['STOP.', 'DO NOT RESTART.'], sentence_count=5)
        self.assertEqual(len(result['sentences']), 2)
        self.assert_grounded(chunk, result)

    def test_empty_and_image_only_chunk_has_empty_summary(self):
        request = {'algorithm': 'lexrank', 'sentence_count': 3, 'language': 'en',
                   'chunks': [{'seq_no': 1, 'lines': []}, {'seq_no': 2, 'lines': [
                       {'line_number': 1, 'line_type': 'image', 'content': 'images/picture.jpg'}]}]}
        result = self.worker.summarize(request)
        self.assertEqual([s['summary_text'] for s in result['summaries']], ['', ''])

    def test_unsupported_language_and_algorithm_fail(self):
        for kwargs in ({'language': 'unsupported'}, {'algorithm': 'invented'}, {'sentence_count': 0}):
            with self.assertRaises(ValueError):
                self.run_chunk(['Example text.'], **kwargs)

    def test_language_detected_for_mixed_metadata(self):
        _, result = self.run_chunk(['设备应保持清洁。'], language='mixed')
        self.assertEqual(result['language'], 'zh')

    def test_lsa_stopword_only_chunk_uses_explicit_fallback(self):
        for text,language in [('For example.', 'en'), ('This is it.', 'en'), ('一、', 'mixed')]:
            chunk,result=self.run_chunk([text],algorithm='lsa',language=language)
            self.assertEqual(result['summary_text'],text)
            self.assertIn(result['ranking_fallback'], {'no_ranking_vocabulary','empty_ranked_result'})
            self.assert_grounded(chunk,result)

    def test_unterminated_chinese_tail_is_retained(self):
        chunk,result=self.run_chunk(['设备应保持清洁。末尾要求不得省略'],language='zh-cn',sentence_count=5)
        self.assertEqual(len(result['sentences']),2)
        self.assertTrue(result['summary_text'].endswith('末尾要求不得省略'))
        self.assert_grounded(chunk,result)


if __name__ == '__main__':
    unittest.main()
