"""FastAPI entry-point for the FHIR analytics service.

Subscribes to NATS subject ``fhir.resource.>`` on startup, maintains an
in-memory :class:`PatientStore`, and computes CMS122 on demand against
the current state. Future iterations swap in TimescaleDB-backed state
and add additional measures.
"""
from __future__ import annotations

import asyncio
import json
import logging
import math
import os
from contextlib import asynccontextmanager
from typing import AsyncIterator

import nats
from fastapi import FastAPI

from .cms122 import compute as compute_cms122
from .history import serialize as serialize_history
from .history import synthesise_history
from .ingest import dispatch
from .measures import MEASURES, compute_all, serialize
from .state import PatientStore

logger = logging.getLogger("analytics-py")
logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(name)s %(message)s")

NATS_URL = os.getenv("NATS_URL", "nats://localhost:4222")
SUBSCRIBE_SUBJECT = "fhir.resource.>"


@asynccontextmanager
async def lifespan(app: FastAPI) -> AsyncIterator[None]:
    store = PatientStore()
    app.state.store = store
    app.state.message_count = 0

    nc = await nats.connect(NATS_URL, name="analytics-py", max_reconnect_attempts=-1)
    app.state.nats = nc
    logger.info("connected to NATS at %s", NATS_URL)

    async def on_message(msg: nats.aio.msg.Msg) -> None:
        # Subject shape is "fhir.resource.<ResourceType>"; trust the
        # publisher to set it correctly rather than re-deriving from
        # the payload (the payload's resourceType must still match).
        parts = msg.subject.split(".")
        resource_type = parts[2] if len(parts) >= 3 else ""
        try:
            resource = json.loads(msg.data)
        except json.JSONDecodeError:
            logger.warning("malformed JSON on subject %s — dropping", msg.subject)
            return
        if resource.get("resourceType") != resource_type:
            logger.warning(
                "subject/payload resourceType mismatch (%s vs %s) — dropping",
                resource_type, resource.get("resourceType"),
            )
            return
        dispatch(store, resource_type, resource)
        app.state.message_count += 1

    sub = await nc.subscribe(SUBSCRIBE_SUBJECT, cb=on_message)
    app.state.subscription = sub
    logger.info("subscribed to %s", SUBSCRIBE_SUBJECT)

    try:
        yield
    finally:
        await sub.unsubscribe()
        await nc.drain()
        logger.info("nats drained")


app = FastAPI(title="analytics-py", version="0.1.0", lifespan=lifespan)


@app.get("/healthz")
def healthz() -> dict[str, object]:
    return {
        "status": "ok",
        "service": "analytics-py",
        "patients_known": len(app.state.store),
        "messages_consumed": app.state.message_count,
    }


@app.get("/measures/cms122/results")
def cms122_results() -> dict[str, object]:
    # Legacy endpoint retained for the existing edge-node BFF + the
    # AppControllerOfflineTests-style integration tests; new clients
    # should use /measures (catalog) and /measures/{id} (per-measure).
    result = compute_cms122(app.state.store)
    return {
        "measureId": "CMS122",
        "measurementPeriod": {
            "start": result.measurement_period_start.isoformat(),
            "end": result.measurement_period_end.isoformat(),
        },
        "denominator": result.denominator,
        "numerator": result.numerator,
        "percentage": result.percentage,
        "gapPatients": [
            {
                "patientId": g.patient_id,
                "age": g.age,
                "latestHbA1c": None if math.isnan(g.latest_hba1c) else g.latest_hba1c,
                "latestHbA1cDate": g.latest_hba1c_date.isoformat(),
            }
            for g in result.gap_patients
        ],
    }


@app.get("/measures")
def list_measures() -> dict[str, object]:
    """All available measures + their current score in one round-trip."""
    return {"measures": [serialize(s) for s in compute_all(app.state.store)]}


@app.get("/measures/{measure_id}")
def measure_detail(measure_id: str) -> dict[str, object]:
    fn = MEASURES.get(measure_id.upper())
    if fn is None:
        return {"error": "unknown measure", "measureId": measure_id}
    return serialize(fn(app.state.store))


@app.get("/measures/{measure_id}/history")
def measure_history(measure_id: str) -> dict[str, object]:
    """Per-measure trend points. See history.py for the data origin."""
    fn = MEASURES.get(measure_id.upper())
    if fn is None:
        return {"error": "unknown measure", "measureId": measure_id}
    score = fn(app.state.store)
    points = synthesise_history(
        measure_id=score.measure_id,
        current_percentage=score.percentage,
        current_denominator=score.denominator,
        direction=score.direction,
    )
    return {
        "measureId": score.measure_id,
        "source": "synthesised",  # honest: not yet TimescaleDB-backed
        "points": serialize_history(points),
    }


@app.get("/scorecard")
def scorecard() -> dict[str, object]:
    """Provider x measure matrix for the heatmap UI."""
    scores = compute_all(app.state.store)
    providers: set[str] = set()
    for s in scores:
        providers.update(s.provider_breakdown.keys())
    provider_list = sorted(providers)
    matrix = []
    for measure in scores:
        row = {
            "measureId": measure.measure_id,
            "title": measure.title,
            "direction": measure.direction,
            "overallPercentage": measure.percentage,
            "overallNumerator": measure.numerator,
            "overallDenominator": measure.denominator,
            "cells": [],
        }
        for pid in provider_list:
            n, d = measure.provider_breakdown.get(pid, (0, 0))
            row["cells"].append({
                "providerId": pid,
                "numerator": n,
                "denominator": d,
                "percentage": round((n / d * 100.0) if d else 0.0, 1),
            })
        matrix.append(row)
    return {"providers": provider_list, "measures": matrix}
