import { Container, getContainer } from "@cloudflare/containers";
import { env } from "cloudflare:workers";

// Runs the Go server (./Dockerfile) as a single Cloudflare Container.
// All application logic lives in Go; this shim only starts the container and
// forwards requests to it.
export class ThomServer extends Container {
  defaultPort = 4000;
  sleepAfter = "10m";
  pingEndpoint = "localhost/ping";

  // Worker secrets and vars are passed to the container on start.
  envVars = {
    ADMIN_USERNAME: env.ADMIN_USERNAME,
    ADMIN_PASSWORD_HASH: env.ADMIN_PASSWORD_HASH,
    JWT_SECRET: env.JWT_SECRET,
    CLIENT_ORIGIN_URLS: env.CLIENT_ORIGIN_URLS,
    SECURE_COOKIES: env.SECURE_COOKIES,
    D1_ACCOUNT_ID: env.D1_ACCOUNT_ID,
    D1_DATABASE_ID: env.D1_DATABASE_ID,
    D1_ENDPOINT: env.D1_ENDPOINT ?? "",
    CF_API_TOKEN: env.CF_API_TOKEN,
    R2_ACCOUNT_ID: env.R2_ACCOUNT_ID,
    R2_BUCKET: env.R2_BUCKET,
    R2_ACCESS_KEY_ID: env.R2_ACCESS_KEY_ID,
    R2_SECRET_ACCESS_KEY: env.R2_SECRET_ACCESS_KEY,
    R2_PUBLIC_BASE_URL: env.R2_PUBLIC_BASE_URL,
    STRIPE_SECRET_KEY: env.STRIPE_SECRET_KEY,
    STRIPE_WEBHOOK_SECRET: env.STRIPE_WEBHOOK_SECRET,
    STRIPE_TAX_ENABLED: env.STRIPE_TAX_ENABLED,
    STRIPE_SHIPPING_CENTS: env.STRIPE_SHIPPING_CENTS,
  };
}

export default {
  async fetch(request, workerEnv) {
    // A single named instance keeps the server's in-memory login throttling
    // and token denylist consistent.
    return getContainer(workerEnv.THOM_SERVER).fetch(request);
  },
};
