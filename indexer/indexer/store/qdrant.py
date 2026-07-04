"""
Qdrant store: collection bootstrap, upsert, semantic search, and the low-level
payload ops the pipeline builds on (scroll/delete by path).
"""
from __future__ import annotations

from dataclasses import dataclass
from typing import Any

from qdrant_client import QdrantClient
from qdrant_client import models as qm


@dataclass(frozen=True)
class SearchHit:
    point_id: str
    score: float
    payload: dict[str, Any]


class QdrantStore:
    def __init__(self, url: str | None = None, *, location: str | None = None) -> None:
        # location=":memory:" is used by tests; url for the real service.
        if location is not None:
            self._client = QdrantClient(location=location)
        else:
            self._client = QdrantClient(url=url)

    # Payload fields indexed for fast filtering. `content_type` distinguishes code
    # from generated domain docs; it is created idempotently below so it is also
    # backfilled onto collections that predate the field.
    _PAYLOAD_INDEXES = (
        ("path", qm.PayloadSchemaType.KEYWORD),
        ("lang", qm.PayloadSchemaType.KEYWORD),
        ("symbol_kind", qm.PayloadSchemaType.KEYWORD),
        ("blob_sha", qm.PayloadSchemaType.KEYWORD),
        ("content_type", qm.PayloadSchemaType.KEYWORD),
    )

    def ensure_collection(self, name: str, dim: int) -> None:
        if not self._client.collection_exists(name):
            self._client.create_collection(
                collection_name=name,
                # on_disk: keep vectors memory-mapped on disk rather than RAM-resident.
                # Across many repos this keeps Qdrant's footprint flat (pages read on
                # demand) instead of growing unbounded with the corpus.
                vectors_config=qm.VectorParams(
                    size=dim, distance=qm.Distance.COSINE, on_disk=True
                ),
            )
        # create_payload_index is idempotent, so this also adds the content_type
        # index to pre-existing collections without a re-embed.
        for field, schema in self._PAYLOAD_INDEXES:
            self._client.create_payload_index(name, field_name=field, field_schema=schema)

    def upsert(self, name: str, points: list[qm.PointStruct]) -> None:
        if points:
            self._client.upsert(collection_name=name, points=points)

    def search(
        self,
        name: str,
        vector: list[float],
        *,
        limit: int,
        lang: str | None = None,
        path_prefix: str | None = None,
        symbol_kind: str | None = None,
        content_type: str | None = None,
    ) -> list[SearchHit]:
        must: list[qm.Condition] = []
        if lang:
            must.append(qm.FieldCondition(key="lang", match=qm.MatchValue(value=lang)))
        if symbol_kind:
            must.append(
                qm.FieldCondition(key="symbol_kind", match=qm.MatchValue(value=symbol_kind))
            )
        if content_type:
            must.append(
                qm.FieldCondition(key="content_type", match=qm.MatchValue(value=content_type))
            )
        result = self._client.query_points(
            collection_name=name,
            query=vector,
            query_filter=qm.Filter(must=must) if must else None,
            limit=limit,
            with_payload=True,
        ).points
        hits = [SearchHit(point_id=str(p.id), score=p.score, payload=p.payload or {}) for p in result]
        if path_prefix:
            hits = [h for h in hits if str(h.payload.get("path", "")).startswith(path_prefix)]
        return hits

    # --- pipeline / GC primitives ---------------------------------------
    def scroll_by_path(self, name: str, path: str, limit: int = 256):
        """Yield all points for a path (its chunks)."""
        offset = None
        flt = qm.Filter(must=[qm.FieldCondition(key="path", match=qm.MatchValue(value=path))])
        while True:
            points, offset = self._client.scroll(
                collection_name=name,
                scroll_filter=flt,
                limit=limit,
                offset=offset,
                with_payload=True,
            )
            for p in points:
                yield str(p.id), (p.payload or {})
            if offset is None:
                break

    def delete_by_path(self, name: str, path: str) -> None:
        """Delete every chunk of a path (used when a path is removed or re-embedded)."""
        self._client.delete(
            collection_name=name,
            points_selector=qm.FilterSelector(
                filter=qm.Filter(
                    must=[qm.FieldCondition(key="path", match=qm.MatchValue(value=path))]
                )
            ),
        )

    def delete_by_content_type(self, name: str, content_type: str) -> None:
        """Delete every point of a content_type (used to fully replace doc points)."""
        self._client.delete(
            collection_name=name,
            points_selector=qm.FilterSelector(
                filter=qm.Filter(
                    must=[
                        qm.FieldCondition(
                            key="content_type", match=qm.MatchValue(value=content_type)
                        )
                    ]
                )
            ),
        )

    def delete(self, name: str, point_ids: list[str]) -> None:
        if point_ids:
            self._client.delete(
                collection_name=name, points_selector=qm.PointIdsList(points=point_ids)
            )

    def count(self, name: str) -> int:
        return self._client.count(collection_name=name).count
