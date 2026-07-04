"""
Periodic reconcile loop — the correctness backstop.

Every ``interval_s`` it reconciles each registered repo (idempotent), picking up
new commits on the primary branch and repairing any drift. It also performs the
initial index on startup. Reconcile runs in a worker thread so the event loop
stays responsive.
"""
from __future__ import annotations

import asyncio
import logging

from ..config import IndexerConfig
from ..service import IndexService

log = logging.getLogger("indexer.scheduler")


class Scheduler:
    def __init__(self, service: IndexService, registry: IndexerConfig, interval_s: int) -> None:
        self._service = service
        self._registry = registry
        self._interval = interval_s
        self._task: asyncio.Task | None = None
        self._stop = asyncio.Event()

    async def _loop(self) -> None:
        while not self._stop.is_set():
            for repo in list(self._registry.repos):
                if self._stop.is_set():
                    break
                try:
                    await asyncio.to_thread(self._service.index_repo, repo.id)
                except Exception as e:  # one repo failing must not kill the loop
                    log.warning("scheduled reindex of %s failed: %s", repo.id, e)
            try:
                await asyncio.wait_for(self._stop.wait(), timeout=self._interval)
            except TimeoutError:
                pass

    def start(self) -> None:
        if self._task is None:
            self._task = asyncio.create_task(self._loop())

    async def stop(self) -> None:
        self._stop.set()
        if self._task is not None:
            await self._task
            self._task = None
