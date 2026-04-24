from parser_base import ParserBackend


def test_parser_backend_cannot_be_instantiated_directly():
    """ParserBackend.parse() must be overridden."""
    backend = ParserBackend()
    try:
        backend.parse("/fake.pdf", "/tmp/out", lambda p, t: None)
        assert False, "Should have raised NotImplementedError"
    except NotImplementedError:
        pass


def test_parser_backend_init_is_noop_by_default():
    """Default init() does nothing and does not raise."""
    backend = ParserBackend()
    backend.init()  # should not raise


def test_subclass_can_override_parse():
    """A subclass that implements parse() works."""

    class FakeParser(ParserBackend):
        name = "fake"

        def parse(self, pdf_path, output_dir, on_progress):
            return {"pages": [], "total_pages": 0, "engine": "fake"}

    parser = FakeParser()
    parser.init()
    result = parser.parse("/fake.pdf", "/tmp/out", lambda p, t: None)
    assert result == {"pages": [], "total_pages": 0, "engine": "fake"}
