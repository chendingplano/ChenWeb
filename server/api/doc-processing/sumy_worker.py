"""Sumy JSON worker embedded in doc-processor. No runtime downloads or DB access."""

import importlib.metadata
import json
import logging
from pathlib import Path
import re
import sys

import nltk
from sumy.models.dom import ObjectDocumentModel, Paragraph, Sentence
from sumy.nlp.tokenizers import Tokenizer
from sumy.summarizers.lex_rank import LexRankSummarizer
from sumy.summarizers.text_rank import TextRankSummarizer
from sumy.summarizers.lsa import LsaSummarizer
from sumy.summarizers.luhn import LuhnSummarizer
from sumy.summarizers.sum_basic import SumBasicSummarizer
from sumy.utils import get_stop_words

ALGORITHMS = {'lexrank': LexRankSummarizer, 'textrank': TextRankSummarizer,
              'lsa': LsaSummarizer, 'luhn': LuhnSummarizer, 'sumbasic': SumBasicSummarizer}
# A provisioned sibling of the project's .venv, without changing user-wide NLTK data.
nltk.data.path.insert(0, str(Path(sys.prefix).parent / 'nltk_data'))


def language_for(raw, text):
    language = (raw or '').strip().lower().replace('_', '-')
    if language in {'zh', 'zh-cn', 'zh-tw', 'zh-hans', 'zh-hant', 'chinese'}:
        return 'zh', 'chinese'
    if language in {'en', 'en-us', 'en-gb', 'english'}:
        return 'en', 'english'
    if language not in {'', 'mixed', 'mul', 'und', 'unknown'}:
        raise ValueError(f'unsupported document language: {raw}')
    chars = [c for c in text if not c.isspace()]
    chinese = sum('\u4e00' <= c <= '\u9fff' for c in chars)
    return ('zh', 'chinese') if chars and chinese / len(chars) > 0.2 else ('en', 'english')


def normalize_with_offsets(text):
    result, positions = [], []
    for match in re.finditer(r'\S+', text):
        if result:
            result.append(' ')
            positions.append(match.start() - 1)
        result.extend(match.group())
        positions.extend(range(match.start(), match.end()))
    return ''.join(result), positions


def summarize_chunk(chunk, language, algorithm, count):
    lines, source, offset = [], [], 0
    for line in chunk['lines']:
        if line['line_type'] in {'image', 'table-image', 'equation-image'} or not line['content'].strip():
            continue
        text = line['content']
        lines.append((line['line_number'], text, offset, offset + len(text)))
        source.append(text)
        offset += len(text) + 1
    text = '\n'.join(source)
    lang, tokenizer_language = language_for(language, text)
    result = {'seq_no': chunk['seq_no'], 'language': lang, 'summary_text': '', 'sentences': []}
    if not text.strip():
        return result
    tokenizer = Tokenizer(tokenizer_language)
    canonical, offsets = normalize_with_offsets(text)
    tokenized = list(tokenizer.to_sentences(canonical))
    # Sumy's Chinese sentence regex omits an unterminated trailing fragment.
    # Preserve every non-whitespace gap/tail, including headings and numeric tables.
    sentences, mappings, cursor = [], [], 0
    for sentence in tokenized:
        normalized, _ = normalize_with_offsets(sentence)
        start = canonical.find(normalized, cursor)
        if start < 0:
            raise ValueError('tokenizer sentence cannot be mapped to source')
        if canonical[cursor:start].strip():
            gap_start = cursor + len(canonical[cursor:start]) - len(canonical[cursor:start].lstrip())
            gap_end = start - len(canonical[cursor:start]) + len(canonical[cursor:start].rstrip())
            sentences.append(Sentence(canonical[gap_start:gap_end], tokenizer))
            mappings.append((gap_start, gap_end))
        end = start + len(normalized)
        sentences.append(Sentence(normalized, tokenizer))
        mappings.append((start, end))
        cursor = end
    if canonical[cursor:].strip():
        start = cursor + len(canonical[cursor:]) - len(canonical[cursor:].lstrip())
        sentences.append(Sentence(canonical[start:], tokenizer))
        mappings.append((start, len(canonical)))
    document = ObjectDocumentModel((Paragraph(sentences),))
    ranker = ALGORITHMS[algorithm]()
    ranker.stop_words = get_stop_words(tokenizer_language)
    if any(ranker.normalize_word(word) not in ranker.stop_words
           for sentence in sentences for word in sentence.words):
        chosen = {id(sentence) for sentence in ranker(document, count)}
    else:
        chosen = set()
        result['ranking_fallback'] = 'no_ranking_vocabulary'
    if not chosen:
        chosen = {id(sentence) for sentence in sentences[:count]}
        result.setdefault('ranking_fallback', 'empty_ranked_result')
    for sentence, (start, end) in zip(sentences, mappings):
        if id(sentence) not in chosen:
            continue
        begin, finish = offsets[start], offsets[end - 1] + 1
        spans = []
        for line_number, line_text, line_start, line_end in lines:
            a, b = max(begin, line_start), min(finish, line_end)
            if a < b:
                spans.append({'line': line_number, 'start': a - line_start, 'end': b - line_start})
        result['sentences'].append({'text': text[begin:finish], 'spans': spans})
    result['summary_text'] = '\n'.join(s['text'] for s in result['sentences'])
    return result


def summarize(request):
    algorithm, count = request['algorithm'], request['sentence_count']
    if algorithm not in ALGORITHMS:
        raise ValueError(f'unsupported Sumy algorithm: {algorithm}')
    if type(count) is not int or not 1 <= count <= 100:
        raise ValueError('sentence_count must be 1..100')
    versions = {package: importlib.metadata.version(package) for package in ('sumy', 'nltk', 'numpy', 'jieba')}
    return {'algorithm': algorithm, 'versions': versions, 'summaries': [
        summarize_chunk(chunk, request.get('language', ''), algorithm, count) for chunk in request['chunks']]}


if __name__ == '__main__':
    logging.basicConfig(level=logging.ERROR)
    try:
        json.dump(summarize(json.load(sys.stdin)), sys.stdout, ensure_ascii=False, allow_nan=False)
        sys.stdout.write('\n')
    except Exception as exc:
        print(f'Sumy extraction failed: {exc}', file=sys.stderr)
        sys.exit(1)
