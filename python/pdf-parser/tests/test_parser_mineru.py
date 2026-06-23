from parser_mineru import annotate_equation_image_paths


def test_annotate_equation_image_paths_from_middle_json():
    content_list = [
        {
            "type": "equation",
            "text": "$$\nE = mc^2\n$$",
            "bbox": [159, 487, 897, 521],
            "page_idx": 10,
        }
    ]
    middle = {
        "pdf_info": [
            {},
            {
                "para_blocks": [
                    {
                        "type": "interline_equation",
                        "lines": [
                            {
                                "spans": [
                                    {
                                        "type": "interline_equation",
                                        "content": "E = mc^2",
                                        "image_path": "equation-11-1.jpg",
                                    }
                                ]
                            }
                        ],
                    }
                ]
            },
        ]
    }

    changed = annotate_equation_image_paths(content_list, middle)

    assert changed == 1
    assert content_list[0]["img_path"] == "images/equation-11-1.jpg"


def test_annotate_equation_image_paths_keeps_existing_img_path():
    content_list = [
        {
            "type": "equation",
            "text": "$$E = mc^2$$",
            "img_path": "images/existing.jpg",
        }
    ]
    middle = {
        "pdf_info": [
            {
                "para_blocks": [
                    {
                        "lines": [
                            {
                                "spans": [
                                    {
                                        "type": "interline_equation",
                                        "content": "E = mc^2",
                                        "image_path": "new.jpg",
                                    }
                                ]
                            }
                        ]
                    }
                ]
            }
        ]
    }

    changed = annotate_equation_image_paths(content_list, middle)

    assert changed == 0
    assert content_list[0]["img_path"] == "images/existing.jpg"


def _write_content_list(base, stem, subdir, mtime):
    import os, json
    d = os.path.join(base, stem, subdir)
    os.makedirs(d, exist_ok=True)
    p = os.path.join(d, f"{stem}_content_list.json")
    with open(p, "w", encoding="utf-8") as f:
        json.dump([{"type": "text", "text": subdir}], f)
    os.utime(p, (mtime, mtime))
    return p


class TestFindContentList:
    def test_selects_newest_not_alphabetical_first(self, tmp_path):
        # Regression: a stale alphabetically-first dir (hybrid_auto) must not win
        # over the freshly written one (ocr). This was the silent-stale-parse bug.
        from parser_mineru import _find_content_list

        base = str(tmp_path)
        stem = "doc1"
        _write_content_list(base, stem, "hybrid_auto", mtime=1000)  # stale
        _write_content_list(base, stem, "hybrid_ocr", mtime=2000)
        fresh = _write_content_list(base, stem, "ocr", mtime=3000)  # newest

        assert _find_content_list(base, stem) == fresh

    def test_raises_when_none_found(self, tmp_path):
        import pytest
        from parser_mineru import _find_content_list

        with pytest.raises(FileNotFoundError):
            _find_content_list(str(tmp_path), "missing")
