// Entry point for the FHIR quality platform edge service. Today it
// serves only /healthz; future iterations add SMART on FHIR launch
// endpoints, CDS Hooks service surface, and BFF aggregation calls to
// the Spring core + Python analytics services.

import Fastify from 'fastify';

const LISTEN_ADDR = process.env.LISTEN_ADDR ?? '0.0.0.0';
const PORT = Number(process.env.PORT ?? 8080);

const app = Fastify({
  logger: { level: process.env.LOG_LEVEL ?? 'info' },
});

app.get('/healthz', async () => ({ status: 'ok', service: 'edge-node' }));

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
