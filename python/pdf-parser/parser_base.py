"""Abstract base class for PDF parser backends."""

from typing import Any, Callable


class ParserBackend:
    """Base class for PDF parser backends.

    Subclasses must set `name` and override `parse()`.
    """

    name: str = ""

    def init(self) -> None:
        """One-time initialization (model loading, JAR verification, etc.)."""

    def parse(
        self,
        pdf_path: str,
        output_dir: str,
        on_progress: Callable[[int, int], None],
    ) -> dict[str, Any]:
        """Parse a PDF file.

        Args:
            pdf_path: Absolute path to the source PDF.
            output_dir: Directory to write output files.
            on_progress: Callback(page_finished, total_pages) called after each
                         page completes.

        Returns:
            {"pages": [...], "total_pages": int, "engine": str}
        """
        raise NotImplementedError
