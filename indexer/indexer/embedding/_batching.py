"""
Request batching for hosted embedding APIs.

Providers cap each request by *number of inputs* and *total tokens*, and cap each
individual input by tokens. A blob can chunk into hundreds of windows, and a
single minified line can be one enormous chunk — so naively sending every chunk
of a file in one call overflows those limits and fails the whole request.

:func:`prepare_batches` truncates each input to a char budget (so one oversized
chunk can't blow a request) and packs inputs into batches that respect both the
item-count and token caps. Order is preserved: concatenating the per-batch
results reconstructs the original input order.
"""
from __future__ import annotations

from collections.abc import Iterable, Iterator

# ~4 chars/token is the standard rough heuristic; we only need an upper-ish bound
# to stay safely under provider caps, not an exact count.
_CHARS_PER_TOKEN = 4


def estimate_tokens(text: str) -> int:
    return max(1, len(text) // _CHARS_PER_TOKEN)


def prepare_batches(
    texts: Iterable[str], *, max_items: int, max_tokens: int, max_chars: int
) -> Iterator[list[str]]:
    """Yield batches of (possibly truncated) texts within the request caps.

    ``max_chars`` must be small enough that a single truncated input fits within
    ``max_tokens`` (i.e. ``max_chars / 4 <= max_tokens``); the caller picks the
    provider's per-input and per-request limits accordingly.
    """
    batch: list[str] = []
    tokens = 0
    for raw in texts:
        text = raw[:max_chars] if len(raw) > max_chars else raw
        cost = estimate_tokens(text)
        if batch and (len(batch) >= max_items or tokens + cost > max_tokens):
            yield batch
            batch, tokens = [], 0
        batch.append(text)
        tokens += cost
    if batch:
        yield batch
