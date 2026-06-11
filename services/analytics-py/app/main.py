"""FastAPI entry-point for the FHIR analytics service.

Today it serves only /healthz; future iterations will add the NATS
JetStream consumer, value-set lookups, eCQM compute (starting with
CMS122), and TimescaleDB writes.
"""
from fastapi import FastAPI

app = FastAPI(title="analytics-py", version="0.1.0")


@app.get("/healthz")
def healthz() -> dict[str, str]:
    return {"status": "ok", "service": "analytics-py"}
