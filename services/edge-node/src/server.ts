// Entry point for the FHIR quality platform edge service.
//
// Public surface:
//
//   GET  /healthz                       liveness for docker-compose
//   GET  /api/measures                  catalog of all eCQMs + scores
//   GET  /api/measures/cms122           per-measure detail (legacy + new)
//   GET  /api/measures/:measureId       per-measure detail (CMS122/125/165/117)
//   GET  /api/scorecard                 provider x measure matrix for the heatmap
//
// The BFF aggregates Python analytics output with Spring Boot measure-
// library metadata into one dashboard-shaped response per route.

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

async function fetchJson<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) {
    throw new Error(`upstream ${url} returned HTTP ${res.status}`);
  }
  return (await res.json()) as T;
}

app.get('/healthz', async () => ({ status: 'ok', service: 'edge-node' }));

// Catalog + scores for every measure in one round-trip — what the
// dashboard hits on first load.
app.get('/api/measures', async (_req, reply) => {
  try {
    return await fetchJson<unknown>(`${ANALYTICS_API_URL}/measures`);
  } catch (err) {
    app.log.error({ err }, 'measures catalog fetch failed');
    return reply.code(502).send({ error: 'upstream aggregation failed' });
  }
});

// Provider × measure matrix for the heatmap.
app.get('/api/scorecard', async (_req, reply) => {
  try {
    return await fetchJson<unknown>(`${ANALYTICS_API_URL}/scorecard`);
  } catch (err) {
    app.log.error({ err }, 'scorecard fetch failed');
    return reply.code(502).send({ error: 'upstream aggregation failed' });
  }
});

// 12-month trend for one measure.
app.get<{ Params: { measureId: string } }>('/api/measures/:measureId/history', async (req, reply) => {
  const id = req.params.measureId.toUpperCase();
  try {
    return await fetchJson<unknown>(`${ANALYTICS_API_URL}/measures/${id}/history`);
  } catch (err) {
    app.log.error({ err, measureId: id }, 'history fetch failed');
    return reply.code(502).send({ error: 'upstream history failed' });
  }
});

// Per-measure detail (full gap list). The path doubles up so the older
// /api/measures/cms122 URL keeps working.
app.get<{ Params: { measureId: string } }>('/api/measures/:measureId', async (req, reply) => {
  const id = req.params.measureId.toUpperCase();
  try {
    const [metadata, result] = await Promise.all([
      fetchJson<MeasureMetadata>(`${CORE_API_URL}/measures/${id}`).catch(() => null),
      fetchJson<{ measureId: string; [k: string]: unknown }>(`${ANALYTICS_API_URL}/measures/${id}`),
    ]);
    return {
      ...result,
      reference: metadata?.reference,
      diagnosisCodes: metadata?.diagnosisCodes,
      labCodes: metadata?.labCodes,
    };
  } catch (err) {
    app.log.error({ err, measureId: id }, 'measure detail fetch failed');
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
