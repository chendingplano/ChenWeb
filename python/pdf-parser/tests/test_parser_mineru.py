from parser_mineru import PhaseTracker, _processing_pages, annotate_equation_image_paths, annotate_list_item_bboxes


def test_phase_tracker_reports_all_nine_phases_and_elapsed_time(monkeypatch):
    ticks = iter([10.0, 16.245, 17.0])
    monkeypatch.setattr("parser_mineru.time.monotonic", lambda: next(ticks))
    tracker = PhaseTracker(system="Linux")

    started, force = tracker.update("Sep 28 02:30:14 Layout Predict:   0%|          | 0/16 [00:00<?, ?it/s]")
    assert force is True
    assert len(started) == 9
    assert started[0] == {"name": "Layout Predict", "progress": 0, "status": "active"}
    assert all(phase["status"] == "pending" for phase in started[1:])

    finished, force = tracker.update("Layout Predict: 100%|██████████| 16/16 [00:06<00:00, 2.49it/s]")
    assert force is True
    assert finished[0]["elapsed_seconds"] == 6.25
    assert finished[0]["progress"] == 100
    assert finished[0]["status"] == "complete"

    active, force = tracker.update("MFR Predict:  55%|█████▌    | 16/29 [00:05<00:04]")
    assert force is False
    assert active[1]["progress"] == 55
    assert active[1]["status"] == "active"
    assert tracker.update("unrelated log line") is None


def test_mac_phase_tracker_maps_each_predict_to_its_preparation(monkeypatch):
    ticks = iter([1.0, 2.0, 104.25, 105.0, 106.0, 344.5, 345.0, 345.1, 346.0, 346.5])
    monkeypatch.setattr("parser_mineru.time.monotonic", lambda: next(ticks))
    tracker = PhaseTracker(system="Darwin")
    assert [phase["name"] for phase in tracker.phases] == [
        "Layout Preparation", "Extract Preparation", "Post Processing", "Processing pages"
    ]
    layout, force = tracker.update("Layout Preparation: 100%|████| 16/16 [00:00<00:00]")
    assert force is True
    assert layout[0]["status"] == "active" and layout[0]["progress"] == 0
    tracker.update("Predict:   0%|          | 0/16 [00:00<?, ?it/s]")
    layout, force = tracker.update("Predict: 100%|████| 16/16 [01:43<00:00]")
    assert force is True
    assert layout[0]["elapsed_seconds"] == 103.25
    extract, force = tracker.update("Extract Preparation: 100%|████| 16/16 [00:00<00:00]")
    assert extract[1]["status"] == "active" and force is True
    tracker.update("Predict:   0%|          | 0/303 [00:00<?, ?it/s]")
    extract, force = tracker.update("Predict: 100%|████| 303/303 [03:58<00:00]")
    assert extract[1]["status"] == "complete" and extract[1]["elapsed_seconds"] == 239.5
    tracker.update("Post Processing:   0%|          | 0/16 [00:00<?, ?it/s]")
    post, _ = tracker.update("Post Processing: 100%|████| 16/16 [00:00<00:00]")
    assert post[2]["status"] == "complete"
    tracker.update("Processing pages:   0%|          | 0/16 [00:00<?, ?it/s]")
    pages, _ = tracker.update("Processing pages: 100%|████| 16/16 [00:00<00:00]")
    assert pages[3]["status"] == "complete"
    assert all(phase["progress"] == 100 for phase in pages)


def test_only_processing_pages_counter_is_a_page_count():
    assert _processing_pages("Predict: 100%|████| 303/303 [03:58<00:00]") is None
    assert _processing_pages("Processing pages: 100%|████| 16/16 [00:00<00:00]") == (16, 16)


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


def _list_middle_json(page_idx, list_texts, list_bbox=(0, 0, 100, 24)):
    """Build a minimal middle_json with one "list" para_block (with its own
    bbox, in a coordinate space deliberately different from content_list's)
    whose sub-blocks (one per item in list_texts) each have a single
    line/span."""
    y = 0
    blocks = []
    for text in list_texts:
        top = y
        bottom = y + 10
        blocks.append(
            {
                "type": "text",
                "lines": [
                    {
                        "bbox": [0, top, 100, bottom],
                        "spans": [{"type": "text", "content": text}],
                    }
                ],
            }
        )
        y += 12
    pages = [{} for _ in range(page_idx)] + [
        {"para_blocks": [{"type": "list", "bbox": list(list_bbox), "blocks": blocks}]}
    ]
    return {"pdf_info": pages}


def test_annotate_list_item_bboxes_assigns_per_item_box():
    # content_list's coordinate space is deliberately scaled and offset
    # relative to middle_json's (content width 40 vs middle width 100;
    # content x/y origin at 20/200 vs middle's 0/0) to prove the per-item
    # boxes get mapped into content_list's space, not pasted in verbatim.
    content_list = [
        {
            "type": "list",
            "list_items": ["6.2.1.1 first item", "6.2.1.2 second item"],
            "bbox": [20, 200, 60, 224],
            "page_idx": 5,
        }
    ]
    middle = _list_middle_json(5, ["6.2.1.1 first item", "6.2.1.2 second item"])

    changed = annotate_list_item_bboxes(content_list, middle)

    assert changed == 2
    bboxes = content_list[0]["list_item_bboxes"]
    assert bboxes == [[20.0, 200.0, 60.0, 210.0], [20.0, 212.0, 60.0, 222.0]]
    # Distinct boxes, not the single shared block-level box.
    assert bboxes[0] != bboxes[1]
    # And not middle_json's raw (unscaled) coordinates either.
    assert bboxes[0] != [0.0, 0.0, 100.0, 10.0]


def test_annotate_list_item_bboxes_unions_wrapped_lines():
    content_list = [
        {
            "type": "list",
            "list_items": ["one", "two wraps across lines"],
            "bbox": [10, 100, 90, 130],
            "page_idx": 0,
        }
    ]
    middle = {
        "pdf_info": [
            {
                "para_blocks": [
                    {
                        "type": "list",
                        # Same coordinate space as content_list's bbox above
                        # (identity transform) so this test isolates the
                        # wrapped-line union logic from the scaling logic,
                        # which has its own dedicated test.
                        "bbox": [10, 100, 90, 130],
                        "blocks": [
                            {
                                "lines": [
                                    {"bbox": [10, 100, 90, 110], "spans": [{"content": "one"}]}
                                ]
                            },
                            {
                                "lines": [
                                    {"bbox": [10, 112, 90, 122], "spans": [{"content": "two wraps"}]},
                                    {"bbox": [10, 123, 60, 130], "spans": [{"content": " across lines"}]},
                                ]
                            },
                        ],
                    }
                ]
            }
        ]
    }

    changed = annotate_list_item_bboxes(content_list, middle)

    assert changed == 2
    assert content_list[0]["list_item_bboxes"][1] == [10.0, 112.0, 90.0, 130.0]


def test_annotate_list_item_bboxes_skips_on_count_mismatch():
    content_list = [
        {
            "type": "list",
            "list_items": ["a", "b", "c"],
            "bbox": [10, 100, 90, 130],
            "page_idx": 0,
        }
    ]
    middle = _list_middle_json(0, ["a", "b"])  # only 2 sub-blocks, content_list has 3

    changed = annotate_list_item_bboxes(content_list, middle)

    assert changed == 0
    assert "list_item_bboxes" not in content_list[0]


def test_annotate_list_item_bboxes_skips_on_text_mismatch():
    content_list = [
        {
            "type": "list",
            "list_items": ["completely different text", "b"],
            "bbox": [10, 100, 90, 124],
            "page_idx": 0,
        }
    ]
    middle = _list_middle_json(0, ["a", "b"])

    changed = annotate_list_item_bboxes(content_list, middle)

    assert changed == 0
    assert "list_item_bboxes" not in content_list[0]


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
