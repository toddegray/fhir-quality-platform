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
from .ingest import dispatch
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
