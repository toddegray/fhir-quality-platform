// Entry point for the FHIR quality platform edge service.
//
// Today it serves:
//   GET /healthz                — liveness for docker-compose
//   GET /api/measures/cms122    — BFF that aggregates the core-spring
//                                 measure metadata with the analytics-py
//                                 measure results into one dashboard-
//                                 shaped response.
//
// Future iterations add SMART on FHIR launch + CDS Hooks service
// endpoints alongside.

import Fastify from 'fastify';

const LISTEN_ADDR = process.env.LISTEN_ADDR ?? '0.0.0.0';
const PORT = Number(process.env.PORT ?? 8080);
const CORE_API_URL = process.env.CORE_API_URL ?? 'http://localhost:8083';
const ANALYTICS_API_URL = process.env.ANALYTICS_API_URL ?? 'http://localhost:8082';

const app = Fastify({
  logger: { level: process.env.LOG_LEVEL ?? 'info' },
});

interface MeasureMetadata {
  id: string;
  title: string;
  description: string;
  direction: string;
  diagnosisCodes: string[];
  labCodes: string[];
  reference: string;
}

interface GapPatient {
  patientId: string;
  age: number;
  latestHbA1c: number | null;
  latestHbA1cDate: string;
}

interface MeasureResult {
  measureId: string;
  measurementPeriod: { start: string; end: string };
  denominator: number;
  numerator: number;
  percentage: number;
  gapPatients: GapPatient[];
}

async function fetchJson<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(`upstream ${url} returned HTTP ${res.status}`);
  }
  return (await res.json()) as T;
}

app.get('/healthz', async () => ({ status: 'ok', service: 'edge-node' }));

app.get('/api/measures/cms122', async (_req, reply) => {
  try {
    const [metadata, result] = await Promise.all([
      fetchJson<MeasureMetadata>(`${CORE_API_URL}/measures/CMS122`),
      fetchJson<MeasureResult>(`${ANALYTICS_API_URL}/measures/cms122/results`),
    ]);
    return {
      id: metadata.id,
      title: metadata.title,
      description: metadata.description,
      direction: metadata.direction,
      reference: metadata.reference,
      measurementPeriod: result.measurementPeriod,
      denominator: result.denominator,
      numerator: result.numerator,
      percentage: result.percentage,
      gapPatients: result.gapPatients,
    };
  } catch (err) {
    app.log.error({ err }, 'cms122 aggregation failed');
    return reply.code(502).send({ error: 'upstream aggregation failed' });
  }
});

const shutdown = async (signal: string): Promise<void> => {
  app.log.info({ signal }, 'shutdown signal received');
  await app.close();
  process.exit(0);
};
process.on('SIGINT', () => { void shutdown('SIGINT'); });
process.on('SIGTERM', () => { void shutdown('SIGTERM'); });

app.listen({ host: LISTEN_ADDR, port: PORT }).catch((err: unknown) => {
  app.log.error({ err }, 'listen failed');
  process.exit(1);
});
